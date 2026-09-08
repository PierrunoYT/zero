# Zero Codebase Audit

- **Audit date:** 2026-09-08
- **Audited revision:** `1b5db17` (`main`)
- **Scope:** the complete Go repository, its build/release automation, and the
  Node wrapper dependencies that ship the Go binary.
- **GitHub tracking check:** 2026-09-08 against upstream `Gitlawb/zero`; 60 open
  issues and 71 open pull requests were reconciled by title and body, and the
  two High findings were submitted through private vulnerability reporting.
- **Change policy:** audit documentation only; no production remediation or
  refactoring was performed.

## Executive assessment

Zero is a capable, test-rich modular monolith with unusually explicit security,
process-lifecycle, and cross-platform controls. Its strongest features are a
typed provider boundary, a centralized tool-result redaction/budget boundary,
native sandbox adapters, workspace-trust gates, atomic state publication, and a
large regression suite. The main risk is not a generally unsafe design; it is a
small set of security boundaries that still validate a pathname and later use
that pathname, a redirect policy that does not explicitly account for custom
authentication headers, and an optional remote-daemon token flag that exposes a
secret through process arguments.

**Health score: 71/100 — serviceable, with priority security hardening needed.**
This is a review rubric, not a coverage percentage or vulnerability probability.

| Dimension | Score | Evidence-based rationale |
|---|---:|---|
| Security | 13/20 | Strong sandbox, trust, redaction, TLS bridge, and secure-publication controls; redirect/pathname authority and token argv need hardening. |
| Architecture & maintainability | 11/20 | Clear packages and interfaces, but composition and state concentrate in three very large files; transitional contracts are duplicated. |
| Correctness & reliability | 15/20 | Extensive validation, bounded provider/process behavior, and atomic stores; updater limits, generic short writes, and cleanup-error reporting need attention. |
| Concurrency & lifecycle | 12/15 | Full race run passed; ownership is generally explicit. CI does not run `-race`, and OAuth/cleanup lifecycles have boundedness/observability gaps. |
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
| 3 | SEC-03 | Medium-high | MCP resource reads canonicalize, then later stat/read by pathname. | A concurrent swap can invalidate the scope decision; static traversal and symlink escapes are already rejected. |
| 4 | SEC-05 | Medium-high | Go update downloads and archive extraction have no byte/entry expansion limits. | A bad or compromised source can consume disk/time; context timeout alone is not a size bound. |
| 5 | SEC-08 | Medium | Remote daemon clients accept bearer tokens in command-line arguments. | Literal tokens can enter shell history/process inspection; TLS, token-file/env input, and token-free link files otherwise provide strong controls. |
| 6 | SEC-04 | Medium | Update extraction uses lexical/pathname containment rather than rooted operations. | A same-account concurrent swap could redirect extraction, but no privilege expansion or static archive-only escape was demonstrated. |
| 7 | SEC-06 | Medium | Release archive and checksum come from the same source without an independent signature. | Checksums detect corruption and mismatches, not replacement of both assets by a compromised publisher/source. |
| 8 | SEC-07 | Medium | OAuth tokens default to mode-0600 plaintext JSON. | Permissions and atomic publication are strong, but local at-rest confidentiality is weaker than the API-key credential store default. |
| 9 | TEST-01 | Medium | CI runs plain `go test ./...`, not the repository's race-enabled `make test`. | Concurrency is extensive; the manual full race run passed, but regressions are not continuously gated. |
| 10 | DEP-01 | Medium | npm audit reports two transitive moderate vulnerable packages through `tuistory`. | Fixes are available; applicability to Zero's helper use was not proven, so this is dependency exposure rather than a confirmed Zero exploit. |

The canonical registry, lower-ranked items, evidence links, and acceptance
criteria are in [Known Issues](KNOWN_ISSUES.md).

## Findings by severity

- **Critical:** none confirmed. The audit found no demonstrated data corruption,
  exploitable unauthenticated remote service, deadlock, or race-detector failure.
- **High (2):** SEC-01 and SEC-02. Both concern an authority boundary with
  plausible credential disclosure or host write impact and require failing
  regressions before remediation.
- **Medium-high (2):** SEC-03 and SEC-05.
- **Medium (12):** SEC-04/06/07/08, TEST-01/02, DEP-01, ARCH-01/02/03/04, and
  REL-01. Preconditions and confidence are explicit in
  [Known Issues](KNOWN_ISSUES.md).
- **Low-medium (7):** CON-01/02, COR-01, QUAL-01, TEST-03/04, and PERF-01.
- **Low (1):** SEC-09. The low and low-medium entries are bounded lifecycle,
  protocol robustness, supply-chain defense, and quality-debt items rather than
  confirmed severe failures.

This produces **24 distinct findings**. CON-03 in the concurrency report is a
cross-classification of SEC-02/03/04 as filesystem interleavings, not a 25th
finding.

## Complete finding report

This table makes the main report self-contained. “Observed” describes the code
or check result; “risk” states the consequence under its prerequisites, not a
claim of demonstrated exploitation. Detailed evidence, test criteria, and
qualifications remain in the linked specialist sections.

| ID | Severity | Observed evidence | Risk and qualification | Recommended action |
|---|---|---|---|---|
| SEC-01 | High | Provider I/O can follow redirects after adapters attach configurable/custom headers ([detail](SECURITY_AUDIT.md#sec-01--cross-origin-redirects-may-retain-custom-authentication-headers)). | An endpoint-controlled cross-origin redirect may receive a non-standard credential header. Go strips recognized sensitive headers in common cases; no production exploit was demonstrated. | Default to same-origin redirects, reject HTTPS downgrade, and rebuild any explicitly allowed cross-origin request from a non-sensitive allowlist. |
| SEC-02 | High | Workspace components are checked with `Lstat`, then write/edit opens the absolute pathname separately ([detail](SECURITY_AUDIT.md#sec-02--workspace-write-tools-have-a-check-to-use-pathname-window)). | A concurrent component swap could redirect an authorized write outside its root, subject to the effective OS sandbox. Static traversal and observed symlinks already fail. | Use rooted, handle-relative traversal/create/replace on every platform; prove the old mechanism with deterministic swap and Windows reparse tests first. |
| SEC-03 | Medium-high | MCP canonicalizes and checks a resource path before a later stat/read by pathname ([detail](SECURITY_AUDIT.md#sec-03--mcp-resource-scope-decision-is-separated-from-file-open)). | A concurrent local swap can invalidate the scope decision and disclose another readable file to the MCP peer. Static traversal, external symlinks, non-regular files, and oversized resources already fail. | Open through a rooted authority, then type-check, limit, and read that same object. |
| SEC-05 | Medium-high | Updater metadata/download streams and archive extraction have no explicit byte, entry, per-file, or total-expansion budgets ([detail](SECURITY_AUDIT.md#sec-05--go-updater-does-not-bound-downloaded-or-expanded-input)). | A bad or compromised source can consume disk, I/O, CPU, and update time. Context deadlines do not bound fast bytes. | Enforce streamed metadata/download and extraction budgets, handle overflow, clean only this run's staging, and never promote partial output. |
| SEC-04 | Medium | Archive names are checked lexically before pathname-based directory, link, and file creation in private staging ([detail](SECURITY_AUDIT.md#sec-04--update-extraction-confinement-is-pathname-based)). | A same-account concurrent swap could redirect extraction. The audit did not demonstrate privilege expansion or a static archive-only escape; private staging and checksum verification reduce exposure. | Extract relative to one rooted destination authority and define a fail-closed archive-symlink policy with native reparse tests. |
| SEC-06 | Medium | The updater obtains an archive and its SHA-256 sibling from the same release source ([detail](SECURITY_AUDIT.md#sec-06--sibling-checksums-do-not-independently-authenticate-releases)). | Checksums catch corruption/mismatch but not replacement of both assets by a compromised publisher/source. | Retain checksums and add independently verifiable signatures or attestations with pinned identity, rotation, and recovery tests. |
| SEC-07 | Medium | Default OAuth storage is an atomic mode-0600 plaintext JSON file ([detail](SECURITY_AUDIT.md#sec-07--oauth-file-storage-defaults-to-plaintext)). | Same-account compromise, backups, or snapshots can expose bearer/refresh tokens. Restrictive directories/files already prevent ordinary cross-user reading. | Introduce a keyring/encrypted automatic default with explicit plaintext opt-in and a reversible, transactional migration. |
| SEC-08 | Medium | Remote daemon `run`, `attach`, and `link` accept a literal bearer token through `--token` ([detail](SECURITY_AUDIT.md#sec-08--remote-daemon-bearer-tokens-are-accepted-in-argv)). | The optional value can enter shell history or process inspection. TLS, constant-time comparison, environment/token-file input, and secret-free link files are existing controls. | Add/document token-file or protected input, deprecate literal argv compatibly, and test that argv, errors, logs, and links remain secret-free. |
| TEST-01 | Medium | CI runs plain tests although `make test` enables `-race` ([detail](CONCURRENCY_AUDIT.md#test-01--race-detection-is-not-a-ci-gate)). | A future memory race can merge despite this audit's full race run passing. | Require a full Linux race job while retaining plain native-platform tests. |
| TEST-02 | Medium | Redirect, pathname-swap, update-limit, daemon-token, short-write, OAuth-shutdown, and cleanup failure mechanisms lack direct adversarial regressions ([detail](TESTING_AUDIT.md#test-02--missing-adversarial-boundary-cases)). | A remediation may test the wrong layer or regress silently. | Add deterministic tests that fail on the unfixed mechanism for the claimed reason before production changes. |
| DEP-01 | Medium | npm audit reports moderate findings in `@hono/node-server@1.19.14` and `hono@4.12.27` through `tuistory@0.10.0` ([detail](TESTING_AUDIT.md#dependency-and-supply-chain-testing)). | Dependency exposure exists, but use of the affected Hono paths by Zero was not established. | Map reachability, test the supported helper/platform matrix, then upgrade or record a time-bounded evidence-backed exception. |
| ARCH-01 | Medium | `tui/model.go`, `agent/loop.go`, and `cli/app.go` are 6,064, 3,487, and 1,589 lines; CLI/TUI dependency fan-out is high ([detail](CURRENT_ARCHITECTURE.md#current-pressure-points)). | Change review, state ownership, regression analysis, and lifecycle reasoning are expensive; size alone is not a correctness defect. | Preserve root facades and extract one behavior-compatible command service, agent collaborator, or feature-owned TUI model per review. |
| ARCH-02 | Medium | Config resolution imports runtime feature packages and performs domain-specific validation ([`resolver.go`](../../internal/config/resolver.go#L1-L17)). | Configuration layering is coupled to the features it configures, increasing fan-out and import-cycle pressure. | Separate parse/layer/trust normalization from narrow cycle-free validators without changing precedence or fail-closed restrictions. |
| ARCH-03 | Medium | Permission names, aliases, parsing, and ordering are mirrored across agent, swarm, specialist, and CLI surfaces ([detail](TARGET_ARCHITECTURE.md#2-canonical-runtime-contracts)). | Vocabulary can drift and unknown values may behave inconsistently across boundaries. | Establish one cycle-free canonical enum/order; convert legacy aliases only at ingress and fail closed on unknown values. |
| ARCH-04 | Medium | `tools.Result` and `agent.ToolResult` overlap and retain transitional fields ([`tools/types.go`](../../internal/tools/types.go#L95-L152), [`agent/types.go`](../../internal/agent/types.go#L73-L128)). | Multiple internal outcomes complicate redaction, persistence, and cross-surface compatibility. | Inventory encodings/callers, choose one registry-finalized internal outcome, and adapt legacy fields only at persistence/external boundaries. |
| REL-01 | Medium | Several deferred or shutdown paths discard or only partly expose cleanup failures ([detail](CONCURRENCY_AUDIT.md#rel-01--stateful-cleanup-errors-are-inconsistently-observable)). | Unlock, persistence, process, or transport cleanup failure can be invisible or lose another actionable cause. | Classify cleanup as stateful/actionable or best-effort; join actionable errors and emit bounded redaction-safe diagnostics for advisory cleanup. |
| CON-01 | Low-medium | MCP registry timeout can return while a client-factory goroutine remains if that implementation ignores context ([detail](CONCURRENCY_AUDIT.md#con-01--mcp-timeout-may-leave-a-goroutine-if-a-factory-ignores-context)). | A nonconforming pluggable factory can leak bounded-per-attempt goroutines/resources. No leak from current built-ins was demonstrated. | Require and test context compliance or add an independently closable/bounded ownership mechanism. |
| CON-02 | Low-medium | Three loopback OAuth servers lack explicit I/O timeouts and do not join or report terminal `Serve`/`Shutdown` results ([detail](CONCURRENCY_AUDIT.md#con-02--oauth-loopback-servers-lack-io-bounds-and-a-joined-lifecycle)). | A local slow-header connection can survive a failed bounded shutdown. Exposure is loopback-only, and state/PKCE controls remain strong. | Add conservative server bounds and idempotent close/wait with observable completion; test cancellation and slow headers. |
| COR-01 | Low-medium | Daemon framing and ACP newline writers do not reject a legal nil-error short write ([detail](TESTING_AUDIT.md#cor-01--daemon-and-acp-writers-assume-complete-writes)). | A generic writer can silently emit a truncated protocol record, although common production writers normally return an error. | Retry to completion or return `io.ErrShortWrite`; add partial and zero-progress writer tests without changing schemas. |
| QUAL-01 | Low-medium | `make deadcode` exits successfully but reports 76 unreachable declarations ([detail](TESTING_AUDIT.md#qual-01--advisory-dead-code-backlog)). | Unclassified dormant/platform/compatibility code raises maintenance cost; bulk deletion could break supported variants. | Classify each result and enforce a no-new-unexplained baseline; remove only separately evidenced dead code. |
| TEST-03 | Low-medium | No Go fuzz targets were found ([detail](TESTING_AUDIT.md#test-03--no-fuzzing)). | Parser and protocol edge cases rely entirely on example-based coverage. | Seed focused fuzzers for high-risk parsers, archive names, protocol frames, and redaction while retaining deterministic regressions. |
| TEST-04 | Low-medium | Node action-summary tests and npm advisory policy are not visible as main-CI gates ([detail](TESTING_AUDIT.md#test-04--node-helper-checks-are-not-visibly-part-of-main-ci)). | A shipped wrapper or lockfile regression can bypass the primary Go quality path. | Add a small lockfile/helper job or document and enforce the separate release gate. |
| PERF-01 | Low-medium | The uncached suite took 3m35s and the race suite 4m11s, dominated by provider tests with real-time waits ([detail](TESTING_AUDIT.md#perf-01--slow-full-suite-feedback)). | Slow feedback discourages frequent full/race execution; no production hot-path defect was established. | Introduce clocks/short test durations while retaining one realistic integration case per timeout class and scenario benchmarks for extracted hot paths. |
| SEC-09 | Low | Release workflow checkouts retain Actions' persisted Git credential by default while CI disables it ([detail](SECURITY_AUDIT.md#sec-09--release-checkouts-retain-workflow-credentials)). | Trusted release steps receive avoidable credential availability; no exfiltration was observed. | Set `persist-credentials: false` unless a documented later Git operation requires it; continue explicit step-scoped publication tokens. |

## Open upstream issue and pull-request reconciliation

The audit findings above use local IDs; they are not GitHub issue numbers. A
live REST API check of upstream `Gitlawb/zero` on 2026-09-08 returned 60 open
issues and 71 open pull requests. Titles and bodies were compared with each
finding's mechanism and acceptance criteria, not matched by broad keywords
alone.

### Private reports for the High findings

Upstream [requires potential vulnerabilities to be reported privately](../../SECURITY.md).
No public security issue was opened. These links are visible only to advisory
participants until maintainers coordinate disclosure.

| Audit finding | Private upstream report | Verification submitted |
|---|---|---|
| SEC-01 | [GHSA-f484-43mf-99v6](https://github.com/Gitlawb/zero/security/advisories/GHSA-f484-43mf-99v6) (`triage`) | An isolated two-server test on `1b5db17` confirmed that a custom `X-Provider-Key` reaches a cross-origin HTTP 307 target. No real provider was tested. |
| SEC-02 | [GHSA-37cg-763q-376p](https://github.com/Gitlawb/zero/security/advisories/GHSA-37cg-763q-376p) (`triage`) | Source-level check/use interleaving, prerequisites, platform qualifications, and the required deterministic component-swap regression were reported. No timing-loop exploit was claimed. |

### Exact active matches

| Audit finding | Upstream tracking | Coverage decision |
|---|---|---|
| TEST-01 | [Issue #939](https://github.com/Gitlawb/zero/issues/939), [PR #940](https://github.com/Gitlawb/zero/pull/940) | Exact. Both identify the missing `make test`/race CI gate; the PR adds a dedicated Ubuntu race job. Requiring that check in branch protection remains an acceptance step outside the diff. |
| PERF-01 | [PR #955](https://github.com/Gitlawb/zero/pull/955) | Exact remediation in progress. It parallelizes isolated provider tests and shrinks retry backoffs during tests while retaining assertions and race coverage. |

### Partial matches that must retain residual scope

| Audit finding | Upstream tracking | Residual audit scope |
|---|---|---|
| SEC-04 | [Issue #920](https://github.com/Gitlawb/zero/issues/920), [PR #943](https://github.com/Gitlawb/zero/pull/943) | The issue reports a stronger static chained-symlink tar escape, and the PR adds pathname `Lstat`/`EvalSymlinks` checks. That addresses the archive-supplied chain but does not establish the audit's rooted, same-object invariant; a concurrent same-account pathname swap remains separate. The audit did not independently reproduce #920, so its own severity remains qualified. |
| QUAL-01 | [Issue #904](https://github.com/Gitlawb/zero/issues/904), [PR #975](https://github.com/Gitlawb/zero/pull/975) | They track a handful of `deadcode -test` wrappers and remove two test exports. The audit's 76 findings came from the repository target using `-test=false`; those production-reachability results still require classification. |

### Related upstream work that is not coverage

| Audit finding | Related item | Why it is not the same finding |
|---|---|---|
| SEC-02 | [Issue #921](https://github.com/Gitlawb/zero/issues/921), [PR #941](https://github.com/Gitlawb/zero/pull/941) | Atomic temp-and-replace prevents partial-file publication, but it does not bind workspace authorization and the final write to one rooted filesystem object. The component-swap TOCTOU remains untracked. |
| SEC-07 | [Issue #937](https://github.com/Gitlawb/zero/issues/937), [PR #1007](https://github.com/Gitlawb/zero/pull/1007) | These fix oversized multi-login keyring blobs, not the default selection of plaintext OAuth file storage. |
| SEC-08 | [PR #685](https://github.com/Gitlawb/zero/pull/685) | This protects a daemon token file from agent/sandbox reads; it does not remove literal bearer tokens from client argv or shell history. |
| SEC-09 | [PR #951](https://github.com/Gitlawb/zero/pull/951) | This hardens workflow token permissions and timeouts but does not set `persist-credentials: false` on release checkouts. |

**Coverage result across channels:** the 2 High findings now have private
reports; 2 other findings have exact public tracking; 2 have partial public
tracking with explicit residuals; and 18 have no active tracking. Neither High
finding has a public issue or PR, as required by upstream security policy.
Potential vulnerabilities must stay in their private advisories until
maintainers coordinate disclosure. For non-security and safely disclosable
residual work, create approved, scoped upstream issues and reference the audit
IDs. The same point-in-time status is recorded in
[Known Issues](KNOWN_ISSUES.md#upstream-github-tracking-status).

## Cross-cutting assessment

### Security

Strong controls include fail-closed project trust gates, policy-versioned
execution approvals, centralized result redaction, default workspace/network
sandboxing, and atomic mode-0600/0700 state publication. Priority work should
make redirect authority and filesystem authority object-bound rather than
pathname/policy-by-convention, and remove literal bearer tokens from argv.
Details: [Security Audit](SECURITY_AUDIT.md).

### Concurrency and lifecycle

The process manager bounds retained processes and output, MCP stdio uses one
reader with synchronized pending calls, registry startup separates concurrent
I/O from deterministic commit, and swarm scheduling uses cancellation plus
wait groups. The full Linux race suite reported no race. Improve continuous
race coverage, bound/join OAuth loopback servers, and make stateful teardown
failures observable. Details:
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
Release checkouts also retain checkout's Git credential by default, a low-
severity defense-in-depth exception to otherwise strong workflow controls.

### Performance

No high-confidence production performance defect was found. Existing controls
include bounded command output, bounded MCP stderr, process-count limits,
provider idle/content watchdogs, deferred MCP schemas, and a CI performance
smoke harness. The main maintainability/performance risks are large hot-path
units and a full test wall time dominated by provider packages (3m35s for the
uncached non-race run in this orb). Preserve and expand scenario benchmarks when
decomposing hot paths; do not optimize from file size alone.

## Priorities, quick wins, and deliberate non-changes

**Ordered priorities:** first reproduce/fix SEC-01 and SEC-02; then remove the
SEC-08 argv secret path; next apply one rooted-I/O boundary to SEC-03/04 and add
SEC-05 limits; then address lifecycle/contracts/architecture and supply-chain
work in the phases in [Migration Plan](MIGRATION_PLAN.md).

**Quick wins:** add `persist-credentials: false` to release checkouts (SEC-09),
add deterministic short-writer tests/failures (COR-01), gate the existing full
race target, and add OAuth server timeouts plus a joined shutdown result. These
are small in code surface, though each still needs its own compatibility test.

**High-risk changes:** rooted cross-platform write/extraction semantics,
permission/result schema consolidation, OAuth-store default migration, release
signature enforcement, and decomposition of agent/TUI/CLI state. Each crosses
platform, persistence, or user-visible contracts and must remain incremental.

**Leave deliberately alone:** the modular-monolith deployment model, narrow
provider interface, centralized tool-result redaction/budgeting, typed execution
contracts, atomic stores, fail-closed trust/sandbox gates, and the root TUI/
`agent.Run` facades. File size alone does not justify replacing these controls.

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
