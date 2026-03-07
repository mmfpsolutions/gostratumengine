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
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sync/atomic"
	"time"

	"github.com/mmfpsolutions/gostratumengine/pkg/logging"
)

// Client is a JSON-RPC 1.0 HTTP client for communicating with blockchain nodes.
type Client struct {
	url      string
	username string
	password string
	client   *http.Client
	logger   *logging.Logger
	reqID    atomic.Uint64
}

// NewClient creates a new RPC client.
func NewClient(host string, port int, username, password string) *Client {
	return &Client{
		url:      fmt.Sprintf("http://%s:%d", host, port),
		username: username,
		password: password,
		client: &http.Client{
			Timeout: 30 * time.Second,
		},
		logger: logging.New(logging.ModuleRPC),
	}
}

func (c *Client) call(method string, params []interface{}) (json.RawMessage, error) {
	id := c.reqID.Add(1)

	req := rpcRequest{
		ID:     id,
		Method: method,
		Params: params,
	}

	body, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("marshaling request: %w", err)
	}

	httpReq, err := http.NewRequest("POST", c.url, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("creating request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.SetBasicAuth(c.username, c.password)

	resp, err := c.client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("sending request: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("reading response: %w", err)
	}

	var rpcResp rpcResponse
	if err := json.Unmarshal(respBody, &rpcResp); err != nil {
		return nil, fmt.Errorf("parsing response: %w", err)
	}

	if rpcResp.Error != nil {
		return nil, rpcResp.Error
	}

	return rpcResp.Result, nil
}

// Ping tests the connection to the node.
func (c *Client) Ping() error {
	_, err := c.GetBlockchainInfo()
	return err
}

// GetBlockchainInfo returns basic blockchain information.
func (c *Client) GetBlockchainInfo() (*BlockchainInfo, error) {
	result, err := c.call("getblockchaininfo", nil)
	if err != nil {
		return nil, err
	}
	var info BlockchainInfo
	if err := json.Unmarshal(result, &info); err != nil {
		return nil, fmt.Errorf("parsing getblockchaininfo: %w", err)
	}
	return &info, nil
}

// GetBlockTemplate requests a new block template from the node.
func (c *Client) GetBlockTemplate(rules []string) (*BlockTemplate, error) {
	params := []interface{}{
		map[string]interface{}{
			"rules": rules,
		},
	}
	result, err := c.call("getblocktemplate", params)
	if err != nil {
		return nil, err
	}
	var tmpl BlockTemplate
	if err := json.Unmarshal(result, &tmpl); err != nil {
		return nil, fmt.Errorf("parsing getblocktemplate: %w", err)
	}
	return &tmpl, nil
}

// SubmitBlock submits a solved block to the network.
func (c *Client) SubmitBlock(blockHex string) error {
	result, err := c.call("submitblock", []interface{}{blockHex})
	if err != nil {
		return err
	}
	// submitblock returns null on success, or an error string
	var resultStr string
	if err := json.Unmarshal(result, &resultStr); err == nil && resultStr != "" {
		return fmt.Errorf("submitblock rejected: %s", resultStr)
	}
	return nil
}

// GetBestBlockHash returns the hash of the best (tip) block.
func (c *Client) GetBestBlockHash() (string, error) {
	result, err := c.call("getbestblockhash", nil)
	if err != nil {
		return "", err
	}
	var hash string
	if err := json.Unmarshal(result, &hash); err != nil {
		return "", fmt.Errorf("parsing getbestblockhash: %w", err)
	}
	return hash, nil
}

// GetBlockHash returns the block hash at a given height.
func (c *Client) GetBlockHash(height int64) (string, error) {
	result, err := c.call("getblockhash", []interface{}{height})
	if err != nil {
		return "", err
	}
	var hash string
	if err := json.Unmarshal(result, &hash); err != nil {
		return "", fmt.Errorf("parsing getblockhash: %w", err)
	}
	return hash, nil
}
