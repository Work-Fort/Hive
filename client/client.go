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

// Client is an HTTP client for the Hive REST API. It authenticates
// every request with a Passport API key under the ApiKey-v1 scheme.
type Client struct {
	http    http.Client
	baseURL string
	apiKey  string
}

// New creates a Client that authenticates with a Passport API key
// (Authorization: ApiKey-v1 <key>). API keys are recognizable by
// the wf-agent_ or wf-svc_ prefix.
//
// Hive's outbound clients are API-key-only — JWTs are reserved for
// browser-routed traffic which never originates here.
func New(baseURL, apiKey string) *Client {
	return &Client{
		http:    http.Client{Timeout: 30 * time.Second},
		baseURL: strings.TrimRight(baseURL, "/"),
		apiKey:  apiKey,
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
	if c.apiKey != "" {
		req.Header.Set("Authorization", "ApiKey-v1 "+c.apiKey)
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
