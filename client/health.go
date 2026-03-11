// SPDX-License-Identifier: GPL-3.0-or-later
package client

import (
	"context"
	"net/http"
)

// Health returns the current health report. This endpoint is unauthenticated.
// The returned HealthReport.Status will be "healthy", "degraded", or
// "unhealthy". Note that the server returns HTTP 218 for degraded — this is a
// non-standard code and is treated as a success (2xx) by the client.
func (c *Client) Health(ctx context.Context) (*HealthReport, error) {
	var out HealthReport
	return &out, c.do(ctx, http.MethodGet, "/v1/health", nil, &out)
}
