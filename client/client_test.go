// SPDX-License-Identifier: GPL-3.0-or-later
package client

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestClient_APIKeySendsApiKeyV1(t *testing.T) {
	gotAuth := ""
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{}`))
	}))
	defer srv.Close()

	c := New(srv.URL, "wf-svc_secret")
	_, _ = c.GetMe(context.Background())

	if gotAuth != "ApiKey-v1 wf-svc_secret" {
		t.Errorf("Authorization = %q, want %q", gotAuth, "ApiKey-v1 wf-svc_secret")
	}
}
