// SPDX-License-Identifier: GPL-3.0-or-later

// Package client provides a Go HTTP client for the Hive REST API.
// It has zero dependencies on internal Hive packages and can be imported
// freely by external consumers.
package client

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// Client is an HTTP client for the Hive REST API.
type Client struct {
	http    http.Client
	baseURL string
	token   string
}

// New creates a new Client. baseURL should be the scheme+host+port of the
// Hive daemon (e.g., "http://127.0.0.1:17000"). token is a Passport JWT or
// API key sent as a Bearer token on every authenticated request.
func New(baseURL string, token string) *Client {
	return &Client{
		http:    http.Client{Timeout: 30 * time.Second},
		baseURL: strings.TrimRight(baseURL, "/"),
		token:   token,
	}
}

// do executes an HTTP request, checks the status, and decodes the JSON
// response body into out (if non-nil). Returns an *APIError for non-2xx
// responses.
func (c *Client) do(ctx context.Context, method, path string, body, out any) error {
	var bodyReader io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return fmt.Errorf("marshal request: %w", err)
		}
		bodyReader = bytes.NewReader(b)
	}

	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, bodyReader)
	if err != nil {
		return fmt.Errorf("build request: %w", err)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	req.Header.Set("Accept", "application/json")
	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}

	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("execute request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return decodeAPIError(resp)
	}

	if out != nil && resp.StatusCode != http.StatusNoContent {
		if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
			return fmt.Errorf("decode response: %w", err)
		}
	}
	return nil
}

// doWithQuery is like do but appends query parameters to the path.
func (c *Client) doWithQuery(ctx context.Context, method, path string, params url.Values, out any) error {
	p := path
	if len(params) > 0 {
		p = path + "?" + params.Encode()
	}
	return c.do(ctx, method, p, nil, out)
}
