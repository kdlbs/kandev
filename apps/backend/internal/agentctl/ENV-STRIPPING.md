## Env stripping for credential-mode agents (`StripEnv`)

Some agents check environment variables to decide their credential mode. For example, `devin acp` checks `ACP_BACKEND`: when set (any value, including empty), it requires protocol-level `authenticate` and refuses local credentials; when unset, it falls back to reading `~/.local/share/devin/credentials.toml` directly. Kandev (which may inherit `ACP_BACKEND` from Windsurf Next) needs the fall-back path, so the variable must be **absent** from the child environment — not just empty.

The strip list flows agent → instance config → process manager via a single slice, mirroring `RequiresProcessKill`:

1. `agents.RuntimeConfig.StripEnv` (set `[]string{"ACP_BACKEND"}` on `DevinACP`).
2. The lifecycle executors copy it into `agentctl.CreateInstanceRequest.StripEnv`.
3. `instance.Manager` stores it on `config.InstanceConfig.StripEnv`.
4. `process.Manager.buildAdapterConfig` iterates the list and calls `utility.RemoveEnvEntry` on `m.cfg.AgentEnv` before that env is used to spawn child processes.

For the one-shot probe/inference path, the strip list is derived from `Runtime().StripEnv` via the shared `agents.StripEnvFor` helper — it is not an independent field on `InferenceConfig`. The derived value is propagated through `InferenceConfigDTO.StripEnv` and applied by `utility.sanitizeEnvForAgent` before spawning the ephemeral subprocess.

To add another agent that needs env vars stripped: set `StripEnv: []string{"VAR_NAME"}` in its `Runtime()` — that's all.

**Agent environment snapshot:** `process.Manager` snapshots `cfg.AgentEnv` at
construction. Tracker Git must not re-read ambient `os.Environ()` at execution
time. Install command gates/shims and `KANDEV_TEST_*` variables before manager
construction, or pass an explicit copied `AgentEnv` snapshot; helpers that create
managers should accept that snapshot so fixtures cannot become stale.

**SSH command shapes:** SSH option rewriting may preserve only direct OpenSSH
commands, including quoted executable paths. Do not insert options after the
first shell word for env/assignment/exec prefixes or custom/plink wrappers;
unsupported shapes must use a documented safe default or fail closed. Cover
direct, quoted, prefixed, and custom forms with focused tests.
