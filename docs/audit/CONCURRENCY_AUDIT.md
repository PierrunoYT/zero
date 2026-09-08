# Concurrency and Lifecycle Audit

This review covers goroutine ownership, cancellation, synchronization, process
and transport lifecycles, boundedness, and cleanup errors at revision `1b5db17`.
The complete Linux race suite result is in [Test Results](TEST_RESULTS.md).

## Executive assessment

Concurrency is extensive (74 production `go` statements by lexical inventory)
but generally intentional. The strongest subsystems document ownership and use
contexts, mutexes, snapshots, bounded channels/buffers, `sync.Once`, and wait
groups. No data race was reported by the full audit run. The principal gaps are
continuous enforcement—CI does not use `-race`—and lifecycle observability when
cleanup is intentionally detached or errors are discarded.

## Ownership map

| Owner | Owned work/resources | Stop/wait behavior | Assessment |
|---|---|---|---|
| Agent run | provider turn session, tool calls, diagnostics | caller context; deferred session close | Close/prewarm errors are deliberately advisory and invisible. |
| Process manager | child command, I/O transport, bounded output, retention goroutine | terminate tree, reap channel, done channel, timeout | Strong bounds and synchronization. |
| MCP stdio client | subprocess, stdin/stdout reader, pending RPC map | fail pending, close stdin, wait 500ms, kill then wait | Clear single-reader and close serialization. |
| MCP registry runtime | per-server connect contexts/clients | concurrent startup, cancel/close; runtime `sync.Once` | Deterministic registration; timed-out context-ignoring factory can leave reaper waiting. |
| MCP SSE client | stream body/cancel and pending calls | cancel/close stream, fail pending | Lock release precedes channel notifications. |
| Swarm scheduler | one timer loop per job | parent/job context + `WaitGroup` | Strong cancel/recheck/non-overlap behavior. |
| Swarm coordinator | task map, colors, change notification | RWMutex and replace-on-change channel | Snapshots avoid pointer escape; wait is context-bound. |
| Daemon pool | bounded worker slots and active handles | drain grace then kill stragglers | Bounded retries/slots; kill errors on drain are advisory. |
| Remote daemon bridge | TLS listener, bounded connection slots, per-connection goroutines | listener close; each handler closes its connection | Handshake is bounded; accepted sessions intentionally clear the deadline and follow daemon protocol lifetime. |
| OAuth loopback callbacks | HTTP listener, one serving goroutine, callback channel | caller timeout + one-second `Shutdown` | Loopback/state controls are strong; server I/O and goroutine completion are not explicitly bounded/joined. |

## Strong controls

### Bounded process execution

The process manager defaults to 64 retained processes, 2 MiB pending output, and
explicit stop/wait ceilings
([`process_manager.go`](../../internal/execution/process_manager.go#L14-L21)).
It stores state under a manager mutex, uses per-process locks and `sync.Once` for
completion, and starts exactly one `Wait` goroutine per command
([`process_manager.go`](../../internal/execution/process_manager.go#L113-L182),
[`process_manager.go`](../../internal/execution/process_manager.go#L364-L400)).
Eviction terminates a live process only after state publication; completed
retention is finite
([`process_manager.go`](../../internal/execution/process_manager.go#L318-L361)).

### MCP transport dispatch

The stdio client serializes writes/id allocation separately from response
dispatch. One lazily started reader owns message reads and routes buffered
responses by ID; cancellation removes pending entries, and terminal read failure
wakes all waiters
([`mcp/client.go`](../../internal/mcp/client.go#L298-L427)). `Close` is serialized,
fails pending calls, closes stdin, waits, kills on a 500ms timeout, and waits for
reaping ([`mcp/client.go`](../../internal/mcp/client.go#L248-L295)). Stderr capture
is mutex-protected and capped at 64 KiB
([`mcp/client.go`](../../internal/mcp/client.go#L103-L137)).

### Deterministic parallel startup

MCP servers connect/list concurrently under per-server timeouts, but workers
write only to unique result slots. Registry validation/conflict detection and
batch publication happen afterward in server order
([`mcp/registry.go`](../../internal/mcp/registry.go#L95-L184)). This avoids both
serial startup latency and nondeterministic shared registry writes.

### Swarm state and scheduling

Coordinator snapshots copy task values under an RWMutex, and `WaitSettled`
takes the settled decision and snapshot in the same lock pass before waiting on
a replace-on-change channel
([`swarm/coordinator.go`](../../internal/swarm/coordinator.go#L216-L288)). The
scheduler derives job contexts from the swarm, tracks loops in a `WaitGroup`,
stops timers on every branch, rechecks cancellation after a simultaneous tick,
and prevents overlapping scheduled members
([`swarm/scheduler.go`](../../internal/swarm/scheduler.go#L110-L143),
[`swarm/scheduler.go`](../../internal/swarm/scheduler.go#L230-L314)).

### Atomic shared persistence

Several stores serialize the entire read-modify-write sequence and publish a
complete temporary file by rename. The OAuth file backend is representative:
mode-0600 random publication under mode-0700 plus a separate lock file
([`oauth/store.go`](../../internal/oauth/store.go#L393-L468)). This prevents
concurrent readers observing partial JSON and concurrent writers losing updates.

## Findings

### TEST-01 — Race detection is not a CI gate

**Severity:** Medium. The Makefile's full `test` target runs `go test ./...
-race -count=1` and says it matches CI expectations
([`Makefile`](../../Makefile#L20-L26)), but the Linux/macOS/Windows CI matrix runs
plain `go test ./...` ([`ci.yml`](../../.github/workflows/ci.yml#L74-L85)). The
audit's full Linux race run passed, which is strong point-in-time evidence but
does not stop future races.

**Recommendation.** Add a Linux race job or invoke the race-enabled target as a
required gate. Keep plain native platform tests for OS-specific behavior. Split
slow provider timing tests if needed, but do not silently narrow race package
coverage.

### CON-01 — MCP timeout may leave a goroutine if a factory ignores context

**Severity:** Low-medium. When connect/list exceeds the timeout, registration
cancels its context and starts a goroutine waiting to receive the eventual
result and close its client
([`mcp/registry.go`](../../internal/mcp/registry.go#L115-L143)). If an injected or
future client factory ignores cancellation and never returns, both its work and
the reaper remain. Startup latency is bounded and current production transports
are context-aware, so this is a lifecycle robustness risk, not evidence of a
present leak under normal adapters.

**Recommendation.** Make context compliance part of the factory contract and
test it. Where an API cannot be interrupted, supervise it with an independently
closable resource or bounded worker strategy; do not claim a timeout fully owns
work it cannot stop.

### REL-01 — Stateful cleanup errors are inconsistently observable

**Severity:** Medium for diagnosis; impact depends on resource.

- Agent provider session close and prewarm errors are explicitly ignored
  ([`agent/loop.go`](../../internal/agent/loop.go#L163-L169)). This is an intentional
  product choice but leaves no structured diagnostic.
- MCP startup closes partial/invalid clients and discards close errors
  ([`mcp/registry.go`](../../internal/mcp/registry.go#L132-L140),
  [`mcp/registry.go`](../../internal/mcp/registry.go#L161-L198)).
- MCP runtime close returns only the first client error rather than joining all
  failures ([`mcp/registry.go`](../../internal/mcp/registry.go#L231-L249)).
- Execution preparation/transport cleanup callbacks are `func()`, so platform
  teardown cannot report failure through the process result
  ([`process_manager.go`](../../internal/execution/process_manager.go#L123-L181),
  [`process_manager.go`](../../internal/execution/process_manager.go#L364-L389)).
- Daemon drain discards straggler kill errors
  ([`daemon/pool.go`](../../internal/daemon/pool.go#L321-L355)).

Routine response-body closes and temporary-file cleanup do not all need to fail
the operation. The issue is lack of a consistent classification between
best-effort hygiene and stateful teardown that can leave processes, locks, or
policy resources behind.

**Recommendation.** Define cleanup classes. Join/return stateful errors when the
caller can act; otherwise emit bounded, redaction-safe structured diagnostics.
Change cleanup callbacks to return errors only at ownership boundaries where
failure matters; avoid noisy blanket handling.

### CON-02 — OAuth loopback servers lack I/O bounds and a joined lifecycle

**Severity:** Low-medium. Three production OAuth callback implementations bind
only to `127.0.0.1` and place the overall login under a caller timeout, but build
`http.Server` without `ReadHeaderTimeout`, `ReadTimeout`, `IdleTimeout`, or an
explicit header cap. Their Serve goroutines discard the terminal error, shutdown
is bounded to one second, shutdown errors are discarded, and no completion
channel is joined
([`oauth/loopback.go`](../../internal/oauth/loopback.go#L30-L55),
[`oauth/loopback.go`](../../internal/oauth/loopback.go#L97-L107),
[`mcp/oauth.go`](../../internal/mcp/oauth.go#L497-L522),
[`provideroauth/openrouter.go`](../../internal/provideroauth/openrouter.go#L72-L109)).
A local slow-header connection can therefore outlive a failed bounded shutdown.

Exposure is local-machine only. The shared listener rejects empty CSRF state and
validates it on callback, MCP uses generated state/PKCE, and OpenRouter uses
PKCE; these controls materially reduce security impact. This is lifecycle and
local resource robustness, not a remotely exposed web-server finding.

**Recommendation.** Configure conservative header/read/idle bounds, retain a
Serve completion result, and make close/wait idempotent and observable. Add
slow-header, caller-cancel, repeated-close, and goroutine-completion tests for
all three flows without weakening loopback/state/PKCE behavior.

### CON-03 — Pathname races are also concurrency correctness issues

SEC-02/03/04 are not Go data races; `-race` cannot detect them. They are
filesystem interleavings between a scope check and kernel path resolution. They
require deterministic adversarial swap tests and rooted APIs. See
[Security Audit](SECURITY_AUDIT.md#sec-02--workspace-write-tools-have-a-check-to-use-pathname-window).

## Locking and channel review conclusions

- No obvious send-on-closed, unlocked shared map, or double-close path was found
  in the reviewed high-concurrency managers.
- Snapshot-before-callback patterns generally avoid holding locks over external
  or blocking work.
- Buffered one-result channels are used where a caller may abandon a producer,
  reducing blocked sends.
- Several subsystems document lock order and single-reader ownership in code;
  preserve those comments when extracting components.
- The race detector passing does not cover cross-process file locks, kernel path
  lookup races, disabled real sandbox integration tests, or macOS/Windows-only
  implementations.

## Recommended concurrency test matrix

1. Required Linux `go test -race ./... -count=1`.
2. Native plain tests on Linux/macOS/Windows, retaining current CI.
3. Focused race stress (`-race -count=20`) for agent parallel tools, MCP clients,
   process manager, daemon session/pool, LSP manager, swarm, and shared stores.
4. Subprocess tests for lock/read-modify-write and process-group teardown.
5. Deterministic filesystem component-swap tests; do not rely on probabilistic
   loops.
6. Leak/ownership tests with a context-ignoring fake at each pluggable transport
   seam.
7. Loopback slow-header and close/wait tests that prove accepted OAuth
   connections cannot survive owner shutdown.

The implementation order and exit gates are in [Migration Plan](MIGRATION_PLAN.md).
