# Target Architecture

This is a proposed incremental target for Zero after the priority findings are
reproduced and fixed. It preserves a Go modular monolith, current user-facing
surfaces, and platform support. It is **not** authorization for a rewrite; the
ordered, reversible sequence is in [Migration Plan](MIGRATION_PLAN.md).

## Goals

1. Bind filesystem and redirect authority at use time, not only during an
   earlier validation step.
2. Keep one canonical vocabulary for permissions, execution, and tool outcomes.
3. Make each long-lived goroutine/process/client have an explicit owner, stop
   signal, wait point, and error policy.
4. Keep composition at the edge while reducing the size of each composition
   unit.
5. Split UI and agent behavior by state machine/capability without changing
   persisted session formats or command behavior.
6. Preserve fast local iteration and cross-platform testability throughout.
7. Keep daemon/ACP/MCP wire compatibility explicit, make protocol writes
   complete-or-error, and keep secret material out of argv.

## Non-goals

- No microservices, alternate implementation language, or plugin ABI rewrite.
- No replacement of Bubble Tea, the provider adapters, or the session store.
- No “big bang” package reorganization.
- No removal of compatibility fields until all producers, persisted forms, and
  consumers are inventoried and migration tests pass.
- No security claim based solely on cleaner package names.

## Proposed dependency direction

```text
+----------------------------- surfaces ------------------------------+
| CLI commands | TUI features | headless/ACP | daemon/cron | MCP serve |
+-------------------------------+-------------------------------------+
                                |
                     +----------v----------+
                     | application services|
                     | run/session/update  |
                     +----+--------+-------+
                          |        |
               +----------+        +----------------+
               |                                    |
       +-------v--------+                  +--------v--------+
       | domain contracts|                 | policy services |
       | run/tool/result |                 | trust/permission|
       +-------+--------+                  +--------+--------+
               |                                    |
       +-------v------------------------------------v--------+
       | adapters: providers, tools, MCP, storage, sandbox, OS |
       +------------------------------------------------------+
```

Dependency rule: surfaces and adapters depend on small domain/application
contracts; domain contracts do not import CLI, TUI, provider catalogs, or OS
adapters. `internal/cli` remains the executable composition edge, but each
subcommand composes a small service rather than sharing one monolithic callback
bag.

## Target components

### 1. Security-bound I/O primitives

Create or extend a single internal rooted-I/O abstraction around `os.Root` and
platform-specific handle semantics. It should support open/read, create/replace,
directory creation, and archive extraction while enforcing every traversed
component. The existing rationale and rooted operations in
[`internal/pathjail/pathjail.go`](../../internal/pathjail/pathjail.go#L1-L23)
are the local precedent. Tool writes, MCP resource reads, and update extraction
should consume this primitive rather than replicate pathname checks.

Provider redirect policy should be equally explicit: same-origin redirects may
carry configured auth; cross-origin redirects either fail or rebuild a request
from a documented non-sensitive header allowlist. The policy belongs beside
provider HTTP I/O, not in each adapter.

### 2. Canonical runtime contracts

Move permission mode/action identifiers into a cycle-free domain package used by
agent, CLI parsing, specialist, swarm, plugins, and persisted adapters. Keep
aliases at ingress only.

Complete the `ToolOutcome` migration so one canonical result crosses the tool ->
agent -> presentation boundary. Compatibility conversion should occur once at
legacy/persistence ingress, not in every accessor. Preserve model view, human
view, artifact, redaction diagnostics, and changed-file evidence.

### 3. Application services at the composition edge

Replace the all-command `appDeps` shape incrementally with small constructors:

- `InteractiveRunService`
- `HeadlessRunService`
- `AuthConfigService`
- `UpdateService`
- `WorkflowGitService`
- `DaemonService`

Names are illustrative; boundaries matter more than types. Each command gets
only its dependencies. Existing injectable function seams can be wrapped before
they are removed, preserving tests.

### 4. Agent state-machine decomposition

Keep `agent.Run` as the stable facade while extracting cohesive internal
collaborators in this order: provider session lifecycle, turn/compaction driver,
permission decision state, tool batch execution, and completion/finalization.
Each extraction must preserve the same event trace and pass golden/behavioral
tests before the next begins.

### 5. TUI feature models

Keep one Bubble Tea program but delegate feature state/update/view to bounded
models (conversation, onboarding/provider setup, permissions, files/plan,
dictation, specialist/swarm, and overlays). The root model should route messages
and own cross-feature navigation, not implement each feature. Do not split views
without first defining state ownership; moving functions alone would only move
the file-size problem.

### 6. Config normalization boundary

Separate parsing/layering/trust merge from domain validation. The config package
should emit normalized values and invoke narrow validators supplied by stable
domain packages or application composition. Preserve current precedence and
fail-closed project/provider-command restrictions
([`internal/config/resolver.go`](../../internal/config/resolver.go#L67-L110)).

### 7. Lifecycle supervision

Adopt a common internal lifecycle convention:

- constructors return owned resources only after successful initialization;
- every background task derives from an owner context;
- owners cancel, close, then wait in a documented order;
- stateful cleanup returns/join errors; intentionally advisory cleanup is
  recorded through structured diagnostics;
- timeouts bound both caller latency and retained goroutines/resources.

This is a convention and a small helper where useful, not a universal framework.

OAuth loopback listeners should apply the same convention: loopback-only bind,
state/PKCE validation, conservative HTTP I/O bounds, and owner close/wait with an
observable terminal Serve result. The daemon remains a supervised multi-process
mode inside the modular-monolith architecture rather than being hidden behind a
false single-process assumption.

## Target invariants

| Boundary | Required invariant |
|---|---|
| Filesystem scope | The same rooted handle used to decide containment is used to open/create the object; no descendant pathname is re-resolved outside it. |
| Provider redirects | Credentials cross origins only under an explicit, tested policy; custom headers are classified, not assumed harmless. |
| Secret input | Bearer tokens and credentials do not appear in process arguments, logs, diagnostics, or persisted link metadata. |
| Update input | Download bytes, archive entry count, per-entry bytes, and cumulative expansion are bounded before promotion. |
| Release authenticity | Artifact authenticity is anchored independently of a sibling checksum asset where the release channel supports it. |
| Permissions | One canonical enum and ordering define all surfaces; unknown values fail closed at ingress. |
| Tool results | One canonical outcome is redacted/budgeted once and adapted only at external compatibility boundaries. |
| Concurrency | Every goroutine is owner-cancelled and owner-waited, or explicitly finite with a tested upper bound. |
| Persistence | Shared state is written completely, synchronized, and atomically replaced; tests redirect all real user directories. |
| Wire protocols | Daemon, ACP, MCP, and stream-JSON records are emitted completely or fail; schema/version compatibility is tested at adapters. |

## Mapping from current to target

| Current element | Target treatment |
|---|---|
| `internal/pathjail` rooted precedent | Extend/adopt as the common confined-I/O boundary. |
| `tools.Registry` output boundary | Retain; make its canonical outcome the only internal representation. |
| `zeroruntime.Provider` | Retain as stable provider interface. |
| `execution` typed contract | Retain; move shared permission vocabulary alongside it or another cycle-free domain package. |
| `cli.appDeps` | Wrap by command-specific services, then shrink after parity tests. |
| `agent.Run` | Retain facade; extract internal state-machine collaborators. |
| TUI root model | Retain root/router; move feature ownership behind bounded model interfaces. |
| config runtime imports | Replace with normalized data plus narrow validators while preserving precedence. |
| legacy `Output`/`Display` fields | Keep only at persistence/API adapters until migration evidence permits removal. |

## Fitness checks

The target is reached incrementally when:

1. swap-driven tests for SEC-02/03/04 fail on old code and pass through rooted
   operations on Linux, macOS, and Windows semantics;
2. cross-origin custom-auth redirect tests cover allow, strip, and reject cases;
3. daemon token inputs are argv-free, OAuth loopback owners wait for Serve, and
   daemon/ACP short-writer tests cannot produce silent truncation;
4. `go list` shows config no longer importing feature adapters, and command
   composition fan-out is distributed without introducing cycles;
5. `agent.Run` and the TUI root retain stable public behavior while no single
   extracted feature owns unrelated state;
6. all race, platform, release, smoke, vulnerability, and performance gates stay
   green after every step;
7. deadcode and compatibility fields decline only with call-site/persistence
   evidence, not through bulk deletion.

The rationale for these choices is recorded in [Decisions](DECISIONS.md).
