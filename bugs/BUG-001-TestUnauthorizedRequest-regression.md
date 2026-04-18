---
id: BUG-001
severity: low
status: open
plan: out-of-scope (Plan 02)
filed-by: qa-tester
date: 2026-04-18
---

# TestUnauthorizedRequest fails due to JWKS stub accepting all tokens

## Summary

`TestUnauthorizedRequest` in `tests/e2e/permissions_test.go` fails because the
JWKS stub introduced in commit `f06d9e9` unconditionally accepts any non-empty
API key, so "invalid-token" is treated as valid.

## Repro

```
mise run e2e
```

Output:
```
=== RUN   TestUnauthorizedRequest
    permissions_test.go:96: invalid token: expected ErrUnauthorized, got <nil>
--- FAIL: TestUnauthorizedRequest (0.21s)
```

## Expected

Requests with a bogus API key should receive HTTP 401, and `client.ErrUnauthorized`
should be returned.

## Actual

The JWKS stub's `POST /v1/verify-api-key` handler returns a valid stubIdentity for
any non-empty key string, so the daemon accepts "invalid-token" as authenticated.

## Root cause

`tests/e2e/jwks_stub_test.go` line 77–89: the handler decodes the request body and,
if the key field is non-empty, returns a canned valid identity without any validation.
There is no allowlist or rejection logic.

## Not caused by Plan 02

This regression predates Plan 02 code. It was introduced with the JWKS stub
(`f06d9e9`). Plan 02 code (`claim`, `release`, `renew`, `pool-list`) is not
responsible.

## Fix suggestion

Add a test-only API key constant (e.g. `testAPIKey = "test-api-key-valid"`) to
the harness and configure the stub to return 401 for anything else. The harness
already issues all requests via a specific client token — that token can be the
allowlisted value.

## Linked task

Task #12 (Fix pre-existing gofmt issues) is already queued for interstitial work.
This bug should be tracked alongside it or as a follow-up after Plan 02 completion.
