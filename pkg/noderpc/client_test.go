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
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
)

// captureNode is a stand-in node that records the getblocktemplate request it
// receives and answers with a minimal template.
func captureNode(t *testing.T) (*Client, *string) {
	t.Helper()
	var gotParams string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		var req struct {
			Method string          `json:"method"`
			Params json.RawMessage `json:"params"`
		}
		if err := json.Unmarshal(body, &req); err != nil {
			t.Errorf("bad request body: %v", err)
		}
		if req.Method != "getblocktemplate" {
			t.Errorf("method = %q, want getblocktemplate", req.Method)
		}
		gotParams = string(req.Params)
		w.Write([]byte(`{"result":{"height":7,"bits":"1d00ffff"},"error":null,"id":1}`))
	}))
	t.Cleanup(srv.Close)

	host, portStr, err := net.SplitHostPort(srv.Listener.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	port, _ := strconv.Atoi(portStr)
	return NewClient(host, port, "u", "p"), &gotParams
}

// Without extra parameters the request is exactly what it was before:
// one object holding the rules.
func TestGetBlockTemplate_RulesOnly(t *testing.T) {
	client, got := captureNode(t)
	tmpl, err := client.GetBlockTemplate([]string{"segwit"})
	if err != nil {
		t.Fatalf("GetBlockTemplate: %v", err)
	}
	if tmpl.Height != 7 {
		t.Errorf("height = %d, want 7", tmpl.Height)
	}
	if want := `[{"rules":["segwit"]}]`; *got != want {
		t.Errorf("params = %s, want %s", *got, want)
	}
}

// DigiByte: the algorithm is sent as the second positional parameter, after
// the request object, which is where DigiByte Core reads it.
func TestGetBlockTemplate_WithAlgorithm(t *testing.T) {
	client, got := captureNode(t)
	if _, err := client.GetBlockTemplate([]string{"segwit", "digidollar-oracle"}, "sha256d"); err != nil {
		t.Fatalf("GetBlockTemplate: %v", err)
	}
	if want := `[{"rules":["segwit","digidollar-oracle"]},"sha256d"]`; *got != want {
		t.Errorf("params = %s, want %s", *got, want)
	}
}
