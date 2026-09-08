# Incremental Remediation Plan

This is a proposed implementation sequence for findings at revision `1b5db17`.
No phase was implemented by this audit. Every phase is intended to be reviewable,
reversible, and independently releasable; production refactoring must not begin
until the owning maintainers approve its scope.

## Principles and release gates

1. Write a regression that fails on the old behavior for the claimed reason.
2. Change one authority/contract boundary at a time; avoid mixed security fixes
   and architecture cleanup.
3. Preserve Linux, macOS, and Windows behavior and native tests.
4. Preserve CLI flags/output, config precedence, session/persistence formats,
   plugin/MCP behavior, and sandbox fail-closed semantics unless a separately
   approved migration says otherwise.
5. Required per-phase gates: format, vet, focused tests (with `-race` for
   concurrency), full tests, build, release smoke, govulncheck, diff hygiene,
   and relevant native/Node/package checks.
6. Roll back only resources created by the current operation. Never report
   success when stateful cleanup or unlock failed.

Upstream triage is recorded in [Known Issues](KNOWN_ISSUES.md). Security fixes
must remain coordinated through their private reports; architecture, release
authenticity, OAuth-storage, lifecycle-contract, and fuzzing choices remain
Ideas discussions until maintainers approve implementation scope.

## Phase 0 — Freeze evidence and add failing regressions

**Goal:** prevent fixes from being accepted for the wrong mechanism.

- Record event/session/output compatibility fixtures for CLI, agent, and TUI.
- Add direct tests for SEC-01 through SEC-05, SEC-08, COR-01, CON-02, and REL-01
  listed in
  [Testing Audit](TESTING_AUDIT.md#test-02--missing-adversarial-boundary-cases).
- Add controllable open/extract seams only in tests or at a narrow existing
  boundary so path swaps are deterministic, not timing loops.
- Run each test against the unfixed implementation and retain the expected
  failure message in its pull request evidence.
- Capture agent turn, TUI render/update, session append, registry result, and MCP
  dispatch performance/allocation baselines.

**Exit gate:** every proposed high/medium-high fix has a red test that reaches
that layer; baseline compatibility fixtures and performance commands are
repeatable. **Rollback:** tests/seams only, no persisted or production behavior.

## Phase 1 — Stop credential and workspace-write boundary exposure

### 1A. SEC-01 redirect policy

- Add one provider-I/O same-origin comparison: scheme, canonical host, and
  effective port.
- Default to reject cross-origin and HTTPS-to-HTTP redirects.
- If product requirements identify a cross-origin case, rebuild from an explicit
  non-sensitive allowlist. Never infer that unknown custom headers are safe.
- Preserve stricter caller-provided `CheckRedirect` behavior and retry
  no-replay semantics.

**Exit gate:** redirect matrix passes; provider adapter tests unchanged; no
credential appears in traces/errors.

### 1B. SEC-02 rooted tool writes

- Extend the existing `pathjail` design or a small cycle-free rooted-I/O package
  with rooted parent creation and create/replace.
- Perform stale-file comparison and final write against bound objects; define
  atomic replacement and mode behavior explicitly.
- Implement native Windows reparse protection for every component and accept the
  correct platform error variants.

**Exit gate:** swap tests pass natively, old static path tests remain, sandbox
policy tests remain fail-closed, and stale-write UX is unchanged. **Rollback:**
retain old implementation behind an internal fallback only until native parity
is proven; do not expose a user “unsafe path” switch.

### 1C. SEC-08 daemon token argv deprecation

- Stop recommending `--token`; add/document token-file and existing environment
  input without placing token bytes in arguments or diagnostics.
- Warn on the legacy flag for a release window, then remove it under the normal
  CLI compatibility policy.
- Keep TLS, constant-time comparison, authentication-before-dispatch, and
  token-free session-link persistence unchanged.

**Exit gate:** process-argument and redaction tests prove no recommended/current
path exposes the token; remote run/attach/link remain compatible through the
documented transition. **Rollback:** retain the deprecated parser for another
release, never fall back to unauthenticated operation.

## Phase 2 — Unify confined I/O and bound update input

### 2A. SEC-03 rooted MCP reads

- Reuse the Phase 1 rooted authority for open/read.
- Validate regular-file type and size on the opened object, then read the same
  object through a hard limit.
- Keep URI and allowed-root behavior compatible.

### 2B. SEC-04 rooted extraction

- Extract all entries relative to the already-open destination.
- Decide archive symlink policy before implementation. Safest default is reject;
  compatibility exceptions require proof that a created link cannot influence
  subsequent traversal.
- Keep staging private and checksum verification before extraction.

### 2C. SEC-05 resource budgets

- Define limits for release metadata, compressed download, entry count, one
  expanded file, and total expansion. Make the constants visible/testable.
- Count actual streamed bytes even without `Content-Length`; check arithmetic
  overflow and declared-size mismatch.
- Abort, close, and remove only this run's staging; never promote partial output.

**Exit gate:** all swap/limit/order cases pass, native package release smoke
passes, and ordinary supported release archives remain installable. **Rollback:**
limits may be raised by reviewed code/config if real releases exceed evidence;
do not silently disable limits.

## Phase 3 — Continuous concurrency and lifecycle guarantees

- Add required Linux `go test -race ./... -count=1`; retain plain native matrix.
- Document MCP client-factory cancellation requirements and test a
  context-ignoring fake. Replace any unbounded ownership with a closable or
  bounded strategy.
- Classify cleanup as stateful/actionable or best-effort. Join stateful failures;
  emit bounded, redaction-safe diagnostics for intentionally advisory cleanup.
- Add header/read/idle bounds and joined Serve completion to all three OAuth
  loopback implementations; preserve loopback binding, state, and PKCE.
- Make daemon/ACP generic writes complete or fail with `io.ErrShortWrite` and
  add deterministic partial-writer tests.
- Stress agent parallel tools, MCP calls, process manager, daemon, LSP, swarm,
  and shared stores under `-race -count=20` in focused jobs as runtime permits.

**Exit gate:** required race gate is stable; no orphan process/client in timeout
tests; multiple cleanup failures retain all actionable causes. **Rollback:** CI
runtime can be optimized by caching/job partitioning, not by dropping package
coverage or making failures advisory.

## Phase 4 — Canonical contracts

### 4A. Permissions (ARCH-03)

- Introduce a cycle-free canonical permission mode/action type and ordering.
- Convert strings/legacy aliases only at CLI/config/persistence ingress.
- Unknown values fail closed. Migrate agent, specialist, swarm, MCP, and plugins
  one consumer at a time.

### 4B. Tool outcome (ARCH-04)

- Inventory every producer/consumer and persisted/session/API encoding of
  `tools.Result` and `agent.ToolResult`.
- Choose the registry-finalized canonical outcome as the internal contract.
- Adapt legacy fields once at external/persistence boundaries; retain read
  compatibility for old sessions for a documented period.

**Exit gate:** one source of truth per vocabulary; compatibility fixtures read
old sessions/config and write only the intended current form; redaction and
output budgeting still happen exactly once. **Rollback:** aliases/converters can
remain longer; never remove persisted fields based only on compile success.

## Phase 5 — Reduce composition and state concentration

### 5A. CLI (ARCH-01)

- Wrap existing callbacks in command-specific application services, beginning
  with low-coupling auth/config/update commands and then headless/interactive.
- Shrink `appDeps` only after each service has parity tests.

### 5B. Agent

- Keep `agent.Run` stable. Extract, one at a time: provider-session lifecycle,
  turn/compaction driver, permission state, tool-batch execution, finalization.
- Compare event traces and persisted session fixtures after each extraction.

### 5C. TUI

- Define feature state ownership before moving code. Delegate bounded models for
  conversation, onboarding/provider setup, permissions, files/plan,
  dictation, specialists/swarm, and overlays.
- Keep one Bubble Tea root/router and cross-feature navigation owner.

### 5D. Config (ARCH-02)

- Separate parse/layer/trust normalization from domain validation through narrow
  validators; preserve exact merge precedence and restrictions.

**Exit gate:** fan-out/change concentration trends downward, no dependency
cycles, behavior fixtures and native/full checks pass, and performance remains
within predeclared tolerances. **Rollback:** each extraction is a facade-preserving
move; revert it independently if parity or performance fails.

## Phase 6 — Supply chain, secrets, and quality debt

1. **DEP-01:** source review found the affected Hono APIs unreachable through
   `tuistory@0.10.0`; refresh the lockfile within compatible ranges and run the
   helper matrix under [issue #1031](https://github.com/Gitlawb/zero/issues/1031).
2. **SEC-06:** select a signing/attestation system whose verification identity is
   independent of mutable sibling assets; implement rotation/recovery and test
   before making verification mandatory.
3. **SEC-07:** add encrypted/keyring auto default, explicit plaintext opt-in,
   transactional migration, rollback, and headless UX.
4. **SEC-09:** disable persisted credentials on every release checkout under
   [issue #1029](https://github.com/Gitlawb/zero/issues/1029); no later Git
   operation currently requires them.
5. **TEST-03/04:** use
   [discussion #1035](https://github.com/Gitlawb/zero/discussions/1035) to select
   bounded fuzz pilots, and gate the shipped action-summary suite under
   [issue #1030](https://github.com/Gitlawb/zero/issues/1030).
6. **QUAL-01:** classify 76 deadcode findings. Remove only separately evidenced
   entries; set a no-new-unexplained baseline.
7. **PERF-01:** replace avoidable real-time waits with clocks/short test values
   while retaining one realistic integration case per timeout class.

**Exit gate:** release authenticity failure is fail-closed and recoverable;
existing secrets migrate without loss; npm findings are fixed or justified;
deadcode/test duration improve without platform or behavioral coverage loss.

## Suggested pull-request breakdown

| PR | Scope | Must not include |
|---|---|---|
| 1 | SEC-01 failing tests | policy implementation |
| 2 | SEC-01 provider redirect policy | filesystem or architecture work |
| 3 | SEC-08 token-file path + legacy warning | unrelated daemon protocol changes |
| 4 | SEC-02 swap tests/rooted write API | MCP/updater migration |
| 5 | SEC-02 tool migration | unrelated tool result cleanup |
| 6 | SEC-03 rooted read migration | extraction changes |
| 7 | SEC-04 tests and rooted extraction | updater limits/signing |
| 8 | SEC-05 limits | signing or OAuth changes |
| 9 | race gate + focused lifecycle contract | large refactors |
| 10 | OAuth loopback bounds/join | OAuth storage migration |
| 11 | protocol short-write correctness | protocol schema changes |
| 12+ | one canonical-contract consumer or one facade extraction | cross-surface rewrites |

The exact ranking and acceptance criteria are in [Known Issues](KNOWN_ISSUES.md);
architectural invariants are in [Target Architecture](TARGET_ARCHITECTURE.md).
