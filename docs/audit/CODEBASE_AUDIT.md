# Zero Codebase Audit

- **Audit date:** 2026-09-08
- **Audited revision:** `1b5db17` (`main`)
- **Scope:** the complete Go repository, its build/release automation, and the
  Node wrapper dependencies that ship the Go binary.
- **Change policy:** audit documentation only; no production remediation or
  refactoring was performed.

## Executive assessment

Zero is a capable, test-rich modular monolith with unusually explicit security,
process-lifecycle, and cross-platform controls. Its strongest features are a
typed provider boundary, a centralized tool-result redaction/budget boundary,
native sandbox adapters, workspace-trust gates, atomic state publication, and a
large regression suite. The main risk is not a generally unsafe design; it is a
small set of security boundaries that still validate a pathname and later use
that pathname, plus a redirect policy that does not explicitly account for
custom authentication headers.

**Health score: 71/100 — serviceable, with priority security hardening needed.**
This is a review rubric, not a coverage percentage or vulnerability probability.

| Dimension | Score | Evidence-based rationale |
|---|---:|---|
| Security | 13/20 | Strong sandbox, trust, redaction, and secure-publication controls; four pathname/redirect boundaries need hardening. |
| Architecture & maintainability | 11/20 | Clear packages and interfaces, but composition and state concentrate in three very large files; transitional contracts are duplicated. |
| Correctness & reliability | 15/20 | Extensive validation, bounded provider/process behavior, and atomic stores; updater limits and cleanup-error reporting are incomplete. |
| Concurrency & lifecycle | 12/15 | Full race run passed; ownership is generally explicit. CI does not run `-race`, and some abandoned/cleanup paths are only best effort. |
| Testing & quality gates | 12/15 | 749 test files and cross-platform CI; no fuzz targets and important adversarial boundary cases are absent. |
| Dependencies, release & performance | 8/10 | Reproducible Go module state, pinned Actions, release smoke/performance jobs; npm audit reports two transitive moderate vulnerabilities and release checksums share the artifact trust source. |

See [Testing Audit](TESTING_AUDIT.md), [Security Audit](SECURITY_AUDIT.md),
[Concurrency Audit](CONCURRENCY_AUDIT.md), and [Test Results](TEST_RESULTS.md)
for the evidence behind each score.

## Repository inventory

Inventory commands used tracked files and `go list ./...`; generated and vendored
fixtures were excluded from the production-line count.

| Metric | Result |
|---|---:|
| Go packages | 91 |
| `cmd/*` programs | 8 |
| top-level `internal/*` directories | 78 |
| production Go files | 626 |
| production Go lines | 192,214 |
| Go test files | 749 |
| Go test lines | 196,687 |
| declared Go dependencies | 16 direct, 15 indirect |
| full selected Go module graph | 53 modules excluding the main module |

The three largest production files are
[`internal/tui/model.go`](../../internal/tui/model.go) (6,064 lines),
[`internal/agent/loop.go`](../../internal/agent/loop.go) (3,487), and
[`internal/cli/app.go`](../../internal/cli/app.go) (1,589). The CLI package has
59 internal-package dependencies; TUI has 39. Those numbers identify change
concentration, not defects by themselves.

## Architecture map

```text
+----------------------+       +----------------------+       +----------------------+
| cmd/zero             |------>| internal/cli         |------>| TUI / exec / ACP /    |
| process entry        |       | composition + route  |       | daemon / cron / serve |
+----------------------+       +----------+-----------+       +----------+-----------+
                                          |                              |
                    +---------------------+------------------------------+
                    |                     |                    |
          +---------v---------+ +---------v---------+ +--------v---------+
          | config + trust    | | sessions + state  | | providers        |
          | policy resolution | | durable JSONL     | | normalized stream|
          +---------+---------+ +---------+---------+ +--------+---------+
                    |                     |                    |
                    +-------------+-------+--------------------+
                                  |
                         +--------v---------+
                         | agent loop       |
                         | turns + approval |
                         +--------+---------+
                                  |
             +--------------------+---------------------+
             |                    |                     |
    +--------v---------+ +--------v---------+ +---------v--------+
    | tools registry  | | MCP/plugins/hooks| | specialist/swarm |
    | redact + budget | | extension gates  | | child execution  |
    +--------+--------+ +------------------+ +------------------+
             |
    +--------v------------------------------+
    | execution contract -> sandbox adapters|
    | -> process manager / filesystem / OS   |
    +----------------------------------------+
```

The detailed call paths, package responsibilities, and dependency hotspots are
in [Current Architecture](CURRENT_ARCHITECTURE.md). The recommended end state is
an incremental modular-monolith evolution, not a rewrite; see
[Target Architecture](TARGET_ARCHITECTURE.md).

## Ranked top 10 issues

The ranks combine impact, plausible exposure, confidence in the observed
mechanism, and remediation urgency. “High” does **not** mean exploitation was
demonstrated. Preconditions are documented in the specialist audits.

| Rank | ID | Severity | Finding | Why it ranks here |
|---:|---|---|---|---|
| 1 | SEC-01 | High | Cross-origin provider redirects may retain custom authentication headers. | Credential disclosure is high impact; redirect following and custom auth headers are both supported, but exploitability depends on redirect and provider configuration. |
| 2 | SEC-02 | High | `write_file`/`edit_file` perform pathname checks before pathname writes. | A concurrently swapped workspace component could redirect an authorized write; a handle-relative primitive already exists elsewhere. |
| 3 | SEC-04 | High | Update extraction uses lexical/pathname containment rather than rooted operations. | A same-user concurrent swap could redirect an updater write; extraction runs on checksum-verified release input in a private temp parent, reducing exposure. |
| 4 | SEC-03 | Medium-high | MCP resource reads canonicalize, then later stat/read by pathname. | A concurrent swap can invalidate the scope decision; static traversal and symlink escapes are already rejected. |
| 5 | SEC-05 | Medium-high | Go update downloads and archive extraction have no byte/entry expansion limits. | A bad or compromised source can consume disk/time; context timeout alone is not a size bound. |
| 6 | SEC-06 | Medium | Release archive and checksum come from the same source without an independent signature. | Checksums detect corruption and mismatches, not replacement of both assets by a compromised publisher/source. |
| 7 | SEC-07 | Medium | OAuth tokens default to mode-0600 plaintext JSON. | Permissions and atomic publication are strong, but local at-rest confidentiality is weaker than the API-key credential store default. |
| 8 | TEST-01 | Medium | CI runs plain `go test ./...`, not the repository's race-enabled `make test`. | Concurrency is extensive; the manual full race run passed, but regressions are not continuously gated. |
| 9 | DEP-01 | Medium | npm audit reports two transitive moderate vulnerable packages through `tuistory`. | Fixes are available; applicability to Zero's helper use was not proven, so this is dependency exposure rather than a confirmed Zero exploit. |
| 10 | ARCH-01 | Medium | Runtime behavior is concentrated in large CLI/TUI/agent units. | It increases review and regression cost and compounds duplicated permission/result concepts; it is an incremental-maintainability issue, not a rewrite mandate. |

The canonical registry, lower-ranked items, evidence links, and acceptance
criteria are in [Known Issues](KNOWN_ISSUES.md).

## Cross-cutting assessment

### Security

Strong controls include fail-closed project trust gates, policy-versioned
execution approvals, centralized result redaction, default workspace/network
sandboxing, and atomic mode-0600/0700 state publication. Priority work should
make redirect authority and filesystem authority object-bound rather than
pathname/policy-by-convention. Details: [Security Audit](SECURITY_AUDIT.md).

### Concurrency and lifecycle

The process manager bounds retained processes and output, MCP stdio uses one
reader with synchronized pending calls, registry startup separates concurrent
I/O from deterministic commit, and swarm scheduling uses cancellation plus
wait groups. The full Linux race suite reported no race. Improve continuous
race coverage and make stateful teardown failures observable. Details:
[Concurrency Audit](CONCURRENCY_AUDIT.md).

### Testing

The test code is slightly larger than production code and exercises all but six
packages directly. CI tests Linux, macOS, and Windows and builds/smokes natively.
Important gaps are no fuzzing, no CI race gate, and no swap-driven regression
tests for the three pathname boundaries or cross-origin custom-auth redirect.
Details: [Testing Audit](TESTING_AUDIT.md).

### Dependencies and release

`go mod verify` and `go mod tidy -diff` passed; there are no `replace` directives.
`govulncheck` results are recorded in [Test Results](TEST_RESULTS.md). GitHub
Actions are commit-SHA pinned, and npm publication uses OIDC/provenance plus a
reviewed environment. The Node lockfile currently resolves vulnerable transitive
Hono packages through `tuistory`; the audit did not change dependency versions.

### Performance

No high-confidence production performance defect was found. Existing controls
include bounded command output, bounded MCP stderr, process-count limits,
provider idle/content watchdogs, deferred MCP schemas, and a CI performance
smoke harness. The main maintainability/performance risks are large hot-path
units and a full test wall time dominated by provider packages (3m35s for the
uncached non-race run in this orb). Preserve and expand scenario benchmarks when
decomposing hot paths; do not optimize from file size alone.

## Audit limitations

- Dynamic verification ran in a Linux x64 orb. macOS/Windows behavior was
  assessed from platform code, tests, and CI configuration, not rerun locally.
- No live provider credentials, real MCP servers, real OS keyrings, or native
  sandbox opt-in smoke tests were used.
- Findings distinguish code mechanism from exploitability. No exploit was
  claimed unless a controlled test demonstrated it; none of the high-severity
  risks was exercised against a real user environment.
- This audit is point-in-time at `1b5db17`; generated dependency advisories and
  release state can change after 2026-09-08.

## Document set

- [Current Architecture](CURRENT_ARCHITECTURE.md)
- [Target Architecture](TARGET_ARCHITECTURE.md)
- [Security Audit](SECURITY_AUDIT.md)
- [Concurrency Audit](CONCURRENCY_AUDIT.md)
- [Testing Audit](TESTING_AUDIT.md)
- [Migration Plan](MIGRATION_PLAN.md)
- [Decisions](DECISIONS.md)
- [Test Results](TEST_RESULTS.md)
- [Known Issues](KNOWN_ISSUES.md)
