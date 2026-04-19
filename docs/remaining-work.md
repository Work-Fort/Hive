# Remaining Work — Hive

Tracks remaining work for the hive project. Items are roughly priority-ordered within each section.

---

## Open

See `docs/remaining-features.md` for outstanding feature work.

---

## Test Coverage Gaps

### Convention: every `t.Skip` must be cross-referenced here

Any conditional `t.Skip` in an e2e or integration test MUST have a corresponding
entry in this section. The entry must name the test, state the condition under
which it skips, and describe the work needed to remove the skip.

A skip with no paper trail is indistinguishable from an accidental omission — and
will be treated as one during future audits. The rationale for this rule is
documented in the architecture reference:

> See `skills/lead/go-service-architecture/references/architecture-reference.md`
> §"Multi-Daemon Test Isolation (Per-Backend)" for the harness pattern and
> the anti-pattern that created this gap.

### Current status

As of 2026-04-18, **no `t.Skip` calls exist** in hive's test suite (unit or e2e).
All tests run unconditionally. This section exists to enforce the paper-trail
convention prospectively — any future skip must be documented here before the
commit lands.
