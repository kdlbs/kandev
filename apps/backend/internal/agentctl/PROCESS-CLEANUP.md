## Process-group cleanup and `RequiresProcessKill`

All ACP agents are launched in their own process group, and shutdown must not
report success while any process in that group is still alive. Some agents —
notably `opencode acp` — keep their HTTP server and an MCP child tree alive after
stdin EOF. For those agents, closing stdin on shutdown is not useful; the process
group has to be killed immediately so the MCP children don't re-parent to init
and leak (GH issue #1247).

The "skip graceful wait" signal flows agent → adapter → process manager via a
single bool:

1. `agents.RuntimeConfig.RequiresProcessKill` (set `true` on `OpenCodeACP`).
2. The lifecycle executors copy it into `agentctl.CreateInstanceRequest.RequiresProcessKill`.
3. `instance.Manager` stores it on `config.InstanceConfig.RequiresProcessKill`.
4. `process.Manager.buildAdapterConfig` forwards it into `adapter.Config` → `shared.Config`.
5. `acp.Adapter.RequiresProcessKill()` returns `cfg.RequiresProcessKill`.
6. On shutdown, `process.Manager.killProcessGroupIfRequired` consults the adapter and, when true, sends SIGKILL to the whole pgid via `killProcessGroup` (`syscall.Kill(-pid, SIGKILL)` on Unix).

`RequiresProcessKill: false` does **not** mean children may be left running. It
only allows the normal graceful path first (stdin close, adapter close, process
wait). After the command leader exits, `waitForProcessExit` still checks the
agent process group and sends SIGTERM/SIGKILL if descendants remain. If `Stop(ctx)`
times out, it also re-runs the pgid SIGKILL fallback.

Non-agent protocol subprocesses must use the same ownership path. LSP servers
start through `process.Manager.StartPipedProcess`, which exposes stdin/stdout to
the bridge while registering the command with `ProcessRunner`; instance teardown
then closes admission and reaps the full process tree on Unix and Windows.
LSP auto-install work holds `Manager.BeginOwnedOperation`; npm and Go installer
commands run through `Manager.CombinedOutput`, so teardown cancels downloads,
drains cache mutations, and reaps installer descendants before resources are
released.

When consuming `exec.Cmd.StdoutPipe` or `StderrPipe`, start the command before
reading and finish every reader before calling `cmd.Wait`; `Wait` closes the
pipes after the command exits. Prefer `CombinedOutput` when separate streaming
is not required, and test cancellation with output large enough to exercise
pipe backpressure so a reader/Wait ordering mistake cannot deadlock teardown.
When wait and draining must proceed concurrently, use an explicitly owned
`os.Pipe` assigned to `cmd.Stderr`, close the parent writer after `Start`, join
the reader with `Wait`, and bound or close the reader on timeout so stderr is
not lost or left blocking teardown.
Lifecycle callbacks that publish process state must complete before releasing
readiness waiters for that same process boundary; test callback-before-waiter
ordering.

To add another agent that needs immediate kill instead of graceful stdin close:
set `RequiresProcessKill: true` in its `Runtime()` config.
