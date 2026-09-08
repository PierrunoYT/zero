# Current Architecture

This document describes the architecture observed at revision `1b5db17`. It is
descriptive, not a proposal. See [Target Architecture](TARGET_ARCHITECTURE.md)
for the intended direction and [Codebase Audit](CODEBASE_AUDIT.md) for scoring.

## System shape

Zero is a single-process Go modular monolith with several executable entry
points and optional child processes. `internal/cli` is the primary composition
root. Lower packages expose typed contracts for providers, execution, tools,
storage, and extension transports. The same core is presented through an
interactive TUI, headless execution, ACP, daemon workers, cron, MCP server mode,
and release/diagnostic commands.

```text
+---------------------------- entry programs -----------------------------+
| zero | zero-release | zero-perf-bench | zero-pr-review | sandbox helpers |
+-------------------------------+-----------------------------------------+
                                |
                       +--------v---------+
                       | internal/cli     |
                       | parse + compose  |
                       +---+----+----+----+
                           |    |    |
                 +---------+    |    +----------------+
                 |              |                     |
          +------v------+ +-----v------+       +------v------+
          | interactive | | headless   |       | service     |
          | tui         | | exec / ACP |       | daemon/MCP  |
          +------+------+
                 |              |                     |
                 +--------------+----------+----------+
                                           |
              +----------------------------v---------------------------+
              | session orchestration: config, trust, provider, agent |
              +----------------------------+---------------------------+
                                           |
                  +------------------------+------------------------+
                  |                        |                        |
          +-------v-------+        +-------v-------+        +-------v-------+
          | tools + MCP   |        | persistence   |        | specialists   |
          | plugins/hooks |        | sessions/state|        | swarm/cron    |
          +-------+-------+        +---------------+        +---------------+
                  |
          +-------v---------------------------------------------------+
          | execution request -> policy/sandbox -> process/filesystem |
          +-----------------------------------------------------------+
```

## Entry and control flows

### Main CLI

[`cmd/zero/main.go`](../../cmd/zero/main.go#L1-L11) returns the exit code from
`cli.Run`. `Run` enters a dependency-injected composition root
([`internal/cli/app.go`](../../internal/cli/app.go#L131-L145)); command routing
selects the interactive default or subcommands
([`app.go`](../../internal/cli/app.go#L287-L503)). The broad `appDeps` seam
([`app.go`](../../internal/cli/app.go#L52-L114)) makes CLI tests hermetic but is
also a concentration point.

### Interactive path

The CLI resolves trust/config, provider, tools, execution/sandbox, extensions,
MCP, specialist/swarm, and session state before constructing TUI options
([`internal/cli/app.go`](../../internal/cli/app.go#L701-L961)). The Bubble Tea
boundary owns terminal setup and shutdown
([`internal/tui/run.go`](../../internal/tui/run.go#L20-L105)). `model.go` then
holds a large share of interactive state transitions.

### Headless path

Headless execution builds a scoped registry and sandbox, registers extensions,
and selects output mode
([`internal/cli/exec.go`](../../internal/cli/exec.go#L220-L375)). It wires agent
events to text/JSON/stream-JSON and invokes the common loop
([`exec.go`](../../internal/cli/exec.go#L650-L875)). ACP follows a parallel
composition path per workspace
([`internal/cli/acp.go`](../../internal/cli/acp.go#L32-L85)).

### Agent/provider/tool path

The loop opens a provider turn session and establishes run state
([`internal/agent/loop.go`](../../internal/agent/loop.go#L149-L190)), partitions
tools and streams normalized provider events per turn
([`loop.go`](../../internal/agent/loop.go#L280-L365)), then decodes and gates tool
calls ([`loop.go`](../../internal/agent/loop.go#L1135-L1215)). Providers conform
to the small streaming interface in
[`internal/zeroruntime/types.go`](../../internal/zeroruntime/types.go#L249-L252).

The registry gives in-flight turns an immutable generation snapshot
([`internal/tools/registry.go`](../../internal/tools/registry.go#L120-L183)). All
run results cross one deferred boundary that scrubs secrets, applies output
budgets, and finalizes typed outcomes
([`registry.go`](../../internal/tools/registry.go#L186-L215)). Command-oriented
tools use the platform-neutral request/state/outcome vocabulary in
[`internal/execution/contracts.go`](../../internal/execution/contracts.go#L1-L21).

## Major subsystem responsibilities

| Area | Primary packages | Observed responsibility |
|---|---|---|
| Composition and commands | `internal/cli`, `cmd/*` | Parse commands, resolve dependencies, select surface, map exit status. |
| Interactive UI | `internal/tui`, `internal/terminalpet` | Bubble Tea model, rendering, onboarding, transcript, panels, input/media. |
| Agent runtime | `internal/agent`, `internal/zeroruntime` | Turn loop, compaction, permissions, parallel calls, normalized provider contract. |
| Providers | `internal/providers/*`, provider catalog/model packages | HTTP/SSE adapters, auth, retries, model discovery and provider selection. |
| Tools | `internal/tools` | Built-ins, schemas, safety declarations, output/redaction boundary, file tracking. |
| Execution/sandbox | `internal/execution`, `internal/sandbox` | Typed command protocol, policy evaluation, OS isolation, process lifecycle. |
| Persistence | `internal/sessions`, `internal/config`, `internal/oauth`, `internal/credstore` | Config, durable event sessions, token/key stores, locks and atomic publication. |
| Extensions | `internal/mcp`, `internal/plugins`, `internal/hooks`, `internal/skills` | External tools/content and trust/permission integration. |
| Multi-agent/background | `internal/specialist`, `internal/swarm`, `internal/background`, `internal/cron` | Child sessions/processes, teams, scheduling, status and retained output. |
| Integration modes | `internal/acp`, `internal/daemon`, `internal/peermsg`, `internal/localcontrol` | Editor RPC, worker pool/control sockets, peer messaging, browser/terminal helpers. |
| Delivery/quality | `internal/release`, `internal/update`, `internal/perfbench`, `internal/agenteval` | Build/package/update, smoke/performance harnesses, evaluation. |

## Dependency shape

`go list -json ./...` at the audited revision produced these internal fan-out
hotspots:

| Package | Direct internal imports |
|---|---:|
| `internal/cli` | 59 |
| `internal/tui` | 39 |
| `internal/tools` | 16 |
| `internal/agent` | 14 |
| `internal/providers` / `internal/zerocommands` | 10 each |

The highest internal fan-in packages were `zeroruntime`, `sandbox`, and
`redaction` (15 importers each), followed by `config` (14), and
`providercatalog`/`modelregistry` (11 each). High fan-in is appropriate for
stable contracts but makes compatibility and test discipline important.

The main layering exception is configuration resolution importing runtime
domains—AIML API, model registry, notify, provider catalog, and sandbox
([`internal/config/resolver.go`](../../internal/config/resolver.go#L1-L17))—and
performing their validation. This keeps one authoritative resolver today but
couples the data/config layer to features it configures.

## State and ownership model

- Config is layered user -> project -> environment -> provider command -> CLI
  overrides, with trust-sensitive restrictions in the merge
  ([`resolver.go`](../../internal/config/resolver.go#L67-L110)).
- Session events are durable JSONL under a session store; exec session creation,
  resume, and fork share this layer
  ([`internal/sessions/exec_session.go`](../../internal/sessions/exec_session.go#L39-L115)).
- Provider calls emit normalized stream events; the agent loop owns turn state.
- Tool registration is snapshot-based; the registry owns the result security and
  output boundary.
- The execution runner owns command preparation; the process manager owns
  retained process identity/output/termination
  ([`internal/execution/process_manager.go`](../../internal/execution/process_manager.go#L34-L43)).
- MCP runtime owns client lifetimes and connect contexts
  ([`internal/mcp/registry.go`](../../internal/mcp/registry.go#L47-L57)).
- TUI and headless surfaces own presentation and persistence callbacks, not
  provider protocol details.

## Current strengths

1. **Narrow provider interface.** Provider-specific HTTP details terminate at a
   normalized stream contract.
2. **Centralized tool-output boundary.** Redaction and model-context budgeting
   do not rely on each tool doing the right thing.
3. **Typed execution vocabulary.** Requests, capabilities, states, denial
   sources, and outcomes are explicit and policy-versioned.
4. **Composition seams.** CLI dependency injection and interfaces permit broad
   hermetic tests despite the large root.
5. **Trust-aware extension loading.** Project MCP/plugins/hooks are excluded
   unless workspace trust is established.
6. **Cross-platform intent is visible.** Platform adapters and native CI matrix
   make Linux/macOS/Windows behavior first-class.

## Current pressure points

1. **Change concentration.** `tui/model.go`, `agent/loop.go`, and `cli/app.go`
   combine many reasons to change. `tui/rendering.go` (2,940 lines),
   `tui/provider_wizard.go` (2,220), and `tui/onboarding.go` (1,885) add nearby
   concentration.
2. **Broad composition contract.** `appDeps` has dozens of callbacks, making the
   whole CLI root an implicit service locator.
3. **Configuration/runtime coupling.** Resolution imports and validates several
   runtime domains instead of accepting validators or normalizing data first.
4. **Duplicated domain identifiers.** Permission strings are canonical in
   [`agent/types.go`](../../internal/agent/types.go#L20-L71) but mirrored to avoid
   cycles in [`swarm/team.go`](../../internal/swarm/team.go#L290-L320) and
   specialist/CLI parsing.
5. **Transitional result representations.** `tools.Result` deliberately keeps
   legacy and canonical outcome fields
   ([`internal/tools/types.go`](../../internal/tools/types.go#L95-L152)), and
   `agent.ToolResult` copies much of that surface
   ([`internal/agent/types.go`](../../internal/agent/types.go#L73-L128)). This is
   migration debt, not an immediate correctness finding.
6. **Dormant surface.** `make deadcode` reported 76 unreachable functions, 60 of
   them in TUI/tools/sandbox. The target is advisory in CI, correctly reflecting
   that some may be compatibility or test seams.

The ranked implications are in [Known Issues](KNOWN_ISSUES.md), and the ordered
decomposition is in [Migration Plan](MIGRATION_PLAN.md).
