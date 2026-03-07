/*
 * Copyright 2026 Scott Walter, MMFP Solutions LLC
 *
 * This program is free software; you can redistribute it and/or modify it
 * under the terms of the GNU General Public License as published by the Free
 * Software Foundation; either version 3 of the License, or (at your option)
 * any later version.  See LICENSE for more details.
 */

package noderpc

import (
	"context"
	"encoding/hex"
	"fmt"
	"sync"

	"github.com/go-zeromq/zmq4"
	"github.com/mmfpsolutions/gostratumengine/pkg/logging"
)

// ZMQSubscriber listens for hashblock notifications from a blockchain node via ZMQ.
type ZMQSubscriber struct {
	endpoint string
	logger   *logging.Logger
	onBlock  func(blockHash string)
	cancel   context.CancelFunc
	wg       sync.WaitGroup
}

// NewZMQSubscriber creates a new ZMQ subscriber for hashblock events.
// The onBlock callback is invoked with the block hash whenever a new block is detected.
func NewZMQSubscriber(endpoint string, onBlock func(blockHash string)) *ZMQSubscriber {
	return &ZMQSubscriber{
		endpoint: endpoint,
		logger:   logging.New(logging.ModuleZMQ),
		onBlock:  onBlock,
	}
}

// Start connects to the ZMQ endpoint and begins listening for hashblock notifications.
func (z *ZMQSubscriber) Start() error {
	ctx, cancel := context.WithCancel(context.Background())
	z.cancel = cancel

	sub := zmq4.NewSub(ctx)
	if err := sub.Dial(z.endpoint); err != nil {
		cancel()
		return fmt.Errorf("connecting to ZMQ endpoint %s: %w", z.endpoint, err)
	}

	if err := sub.SetOption(zmq4.OptionSubscribe, "hashblock"); err != nil {
		sub.Close()
		cancel()
		return fmt.Errorf("subscribing to hashblock topic: %w", err)
	}

	z.logger.Info("ZMQ subscriber connected to %s", z.endpoint)

	z.wg.Add(1)
	go func() {
		defer z.wg.Done()
		defer sub.Close()
		z.listen(ctx, sub)
	}()

	return nil
}

func (z *ZMQSubscriber) listen(ctx context.Context, sub zmq4.Socket) {
	for {
		msg, err := sub.Recv()
		if err != nil {
			select {
			case <-ctx.Done():
				z.logger.Info("ZMQ subscriber shutting down")
				return
			default:
				z.logger.Error("ZMQ recv error: %v", err)
				return
			}
		}

		// ZMQ hashblock message has 3 frames: topic, body (32-byte hash), sequence
		if len(msg.Frames) < 2 {
			continue
		}

		topic := string(msg.Frames[0])
		if topic != "hashblock" {
			continue
		}

		blockHash := hex.EncodeToString(msg.Frames[1])
		z.logger.Debug("ZMQ hashblock received: %s", blockHash)
		z.onBlock(blockHash)
	}
}

// Stop disconnects the ZMQ subscriber.
func (z *ZMQSubscriber) Stop() {
	if z.cancel != nil {
		z.cancel()
	}
	z.wg.Wait()
	z.logger.Info("ZMQ subscriber stopped")
}
