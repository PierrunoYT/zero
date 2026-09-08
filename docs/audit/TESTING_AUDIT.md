# Testing, Dependency, and Performance Audit

This document assesses test structure, CI/release quality gates, dependency
hygiene, compatibility, and performance evidence at revision `1b5db17`. Exact
commands and outcomes are recorded in [Test Results](TEST_RESULTS.md).

## Test inventory

| Metric | Observed value |
|---|---:|
| Go test files | 749 |
| Go test lines | 196,687 |
| top-level `Test*`/`Example*` functions (lexical count) | 6,260 |
| production Go lines | 192,214 |
| benchmark functions outside testdata | 4 |
| fuzz targets | 0 |
| packages from `go list ./...` | 91 |
| packages with no direct Go test files | 6 |

The six packages without direct tests are thin command wrappers
(`cmd/zero`, `cmd/zero-linux-sandbox`, `cmd/zero-seccomp`,
`cmd/zero-windows-command-runner`, `cmd/zero-windows-sandbox-setup`) and
`internal/reltime`. Command behavior is substantially exercised through
dependency-injected command packages and release smoke tests, so this is not the
same as six untested major subsystems.

## Strengths

1. **Large colocated regression suite.** Test code slightly exceeds production
   lines and is colocated by package.
2. **Hermetic seams.** CLI uses injected dependencies and returns an exit code
   instead of exiting in tests
   ([`cli/app.go`](../../internal/cli/app.go#L52-L134)). Provider, MCP, execution,
   process, and storage seams support controlled fakes.
3. **Cross-platform CI.** Native Linux, macOS, and Windows jobs run tests, build,
   and smoke the binary
   ([`ci.yml`](../../.github/workflows/ci.yml#L10-L85)). Release packaging uses
   five native OS/architecture runners
   ([`release-artifacts.yml`](../../.github/workflows/release-artifacts.yml#L14-L31)).
4. **Security regression coverage.** Static traversal/symlink cases, trust
   gates, sandbox plans, permission persistence, redaction, update promotion,
   and secret-safe traces have dedicated tests.
5. **Concurrency-specific tests exist.** Examples include daemon send/close race
   coverage, LSP interleavings, concurrent permission/store operations, process
   manager behavior, and scheduler cancellation.
6. **Build/release gates.** CI vets, formats, tests, builds, smokes, scans Go
   vulnerabilities, checks release checksum sets, and runs a performance smoke.
7. **Reproducible modules.** `go.mod` has 16 direct and 15 declared indirect
   requirements, no `replace` directives, and a checked-in `go.sum`.

## Gaps and findings

### TEST-01 — No required race job

The repository's `make test` runs the full race suite
([`Makefile`](../../Makefile#L20-L26)), while CI uses plain tests
([`ci.yml`](../../.github/workflows/ci.yml#L74-L85)). The audit race run passed,
but the repository has 74 lexical production goroutine launches and broad
cross-process behavior. Make a Linux full-race job required; preserve native
plain tests on the other platforms. See [Concurrency Audit](CONCURRENCY_AUDIT.md).

### TEST-02 — Missing adversarial boundary cases

The following tests should be added before remediation so they are proven to
fail for the intended reason:

- provider redirect with `X-Api-Key`/custom auth crossing origins (SEC-01);
- component swap exactly between tool write validation and open (SEC-02);
- MCP resource component swap between canonicalization and read (SEC-03);
- update extraction component swap and symlink/reparse entry ordering (SEC-04);
- chunked oversized updater body, compression bomb, too many entries, and
  cleanup/no-promotion assertions (SEC-05);
- multiple teardown failures retained/joined or diagnostically reported
  (REL-01).

Static invalid-input tests are not substitutes for check/use interleavings.
Tests should call the vulnerable layer directly if earlier guards would reject
the fixture.

### TEST-03 — No fuzzing

No `Fuzz*` target was found. High-value parsers are checksum files, stream JSON,
SSE framing, MCP JSON-RPC IDs/messages, archive names, provider URL/config
normalization, redaction/control-byte normalization, and shell/permission
parsers. Seed with existing regression cases and keep deterministic unit tests
for every discovered bug. Fuzzing complements—not replaces—platform and
interleaving tests.

### QUAL-01 — Advisory dead-code backlog

`make deadcode` exited successfully but reported 76 unreachable functions: 28
in TUI, 16 tools, 16 sandbox, and 16 elsewhere. The CI target is intentionally
advisory
([`ci.yml`](../../.github/workflows/ci.yml#L139-L154)). Classify each finding as
platform entry, compatibility/test seam, dormant feature, or removable code.
Do not bulk-delete platform or migration surfaces based only on Linux
whole-program reachability.

### TEST-04 — Node helper checks are not visibly part of main CI

The audit's `node --test scripts/action-summary.test.mjs` passed all 12 tests,
but the reviewed CI workflow has no Node test step. Likewise `npm audit` is not a
gate. Add a small lockfile/Node job if the wrapper/action remains a shipped
surface. Keep vulnerability scanning separate from applicability decisions so a
temporary advisory does not encourage blind dependency changes.

### PERF-01 — Slow full-suite feedback

The uncached non-race suite took 3m35s in this orb; provider packages reported
120s (`anthropic`) and 180s (`openai`) package times. This does not indicate slow
production provider behavior, but it raises iteration cost. Identify intentional
timeout tests and use injectable clocks/short test constants where semantics can
remain faithful. Preserve at least one real-time integration test for each
timeout class.

## Coverage assessment

A numeric statement-coverage percentage was not used as the health score.
Coverage can reward broad happy-path execution while missing the exact security
interleavings in this audit. The stronger observed evidence is:

- nearly all packages have direct tests;
- behavior and platform branches have many explicit regression tests;
- full plain and race executions completed;
- specific critical negative paths remain untested as listed in TEST-02.

Future coverage reports should be package-differential and paired with mutation
or “fails without fix” evidence for security changes. Avoid a repository-wide
percentage gate that incentivizes low-value assertions.

## Dependency and supply-chain testing

### Go

- `go mod verify`: passed.
- `go mod tidy -diff`: passed with no changes.
- `make vulncheck`: see [Test Results](TEST_RESULTS.md).
- GitHub's security job makes `govulncheck` a hard gate and keeps deadcode/static
  lint advisory ([`ci.yml`](../../.github/workflows/ci.yml#L117-L154)).
- There is no dependency-update automation file in `.github`; whether updates
  are intentionally manual is not documented in the reviewed configuration.

### Node/npm

The lockfile resolves `tuistory@0.10.0` ->
`@hono/node-server@1.19.14`/`hono@4.12.27`. npm audit reported:

- GHSA-frvp-7c67-39w9, encoded-backslash static path traversal on Windows;
- GHSA-8j4g-w8fx-2239, CORS middleware ReDoS;
- GHSA-f23p-vx2j-j53r, memoized SSR cross-user disclosure;
- GHSA-79qm-7rj5-m7r9, proxy `Connection` header handling (low advisory rolled
  into the moderate package result);
- GHSA-54fx-42gc-7vw4, language middleware algorithmic complexity DoS.

Audit metadata reports fixes available. These are transitive helper dependencies
and affected API reachability was not established; perform an upgrade and helper
smoke matrix, then document dismissals only with call-path evidence. The audit
did not modify `package-lock.json`.

### Release

Actions are commit-SHA pinned, checkout generally disables persisted credentials,
artifacts are built/smoked natively, checksum sets are verified, and npm uses
OIDC trusted publishing/provenance with a required-reviewer environment
([`release-artifacts.yml`](../../.github/workflows/release-artifacts.yml#L33-L81),
[`release-artifacts.yml`](../../.github/workflows/release-artifacts.yml#L108-L145)).
Add an independent signature/attestation verification test for downloaded GitHub
release assets; sibling SHA-256 files remain useful but share the source.

## Compatibility assessment

- The Go toolchain is pinned by `go 1.26.6`; CI and Make versioned tools read
  `go.mod`, avoiding a parallel version declaration.
- CI covers Linux/macOS/Windows. Release artifacts cover Linux x64/arm64, macOS
  x64/arm64, and Windows x64.
- Many tests contain platform-specific skip logic; native matrix execution is
  therefore essential. This Linux audit cannot validate skipped native branches.
- Host `git` is invoked throughout Zero. Any future new flag/subcommand must
  document its minimum exact-command version and provide a fallback/gate, per
  repository policy.
- Transitional `tools.Result` and `agent.ToolResult` fields are compatibility
  evidence; removal needs persisted-session and all-surface tests.

## Performance assessment

No high-confidence production bottleneck was established from static review.
Controls worth preserving include:

- provider idle and heartbeat-without-content watchdogs
  ([`providerio.go`](../../internal/providers/providerio/providerio.go#L23-L83));
- deferred MCP tool schemas to reduce repeated prompt tokens
  ([`config/resolver.go`](../../internal/config/resolver.go#L56-L65));
- process count/output bounds
  ([`process_manager.go`](../../internal/execution/process_manager.go#L14-L21));
- MCP stderr and resource read bounds;
- registry-wide model output ceilings and spill artifacts;
- a checked-in CI performance smoke
  ([`ci.yml`](../../.github/workflows/ci.yml#L87-L115));
- focused benchmarks for transcript rendering, model lookup, and large tool I/O.

Before architectural extraction, record baseline allocations/latency for agent
turn processing, TUI update/render, registry finalization, session append, and
MCP dispatch. Compare distributions, not one microbenchmark number. Performance
reports remain evidence artifacts and should not be committed, per repository
policy.

## Recommended gate order

1. Fast: format, focused tests, vet on touched packages.
2. Required pre-merge: full plain tests, Linux full race, build, smoke, diff
   hygiene, govulncheck.
3. Advisory-to-required after backlog classification: static lint and zero
   introduced deadcode.
4. Shipped wrapper: Node tests and npm audit/applicability record.
5. Security-boundary changes: deterministic negative test proven failing before
   fix, native path semantics, focused race where relevant.
6. Release changes: native package/verify/smoke plus signature/provenance and
   downloader limit tests.

See [Migration Plan](MIGRATION_PLAN.md) for sequencing and
[Known Issues](KNOWN_ISSUES.md) for acceptance criteria.
