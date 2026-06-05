import type { ExecutorProfile } from "@/lib/types/http";
import type { RemoteAuthSpec } from "@/lib/api/domains/settings-api";
import { createDebugLogger } from "@/lib/debug/log";

const debug = createDebugLogger("executor-compat");

const REMOTE_EXECUTOR_TYPES = new Set(["local_docker", "remote_docker", "sprites"]);

export function executorRequiresAgentCredentials(executorType?: string | null): boolean {
  if (!executorType) return false;
  return REMOTE_EXECUTOR_TYPES.has(executorType);
}

function parseJSON<T>(raw: string | undefined, fallback: T): T {
  if (!raw) return fallback;
  try {
    return JSON.parse(raw) as T;
  } catch {
    return fallback;
  }
}

/**
 * Reasons a compat check resolved the way it did. Surfaced via the
 * `[executor-compat]` debug logger so triage of the task-create dialog's
 * "No compatible agent profiles" state is greppable.
 *   executor-local: executor doesn't need per-agent creds → allowed
 *   no-spec:        agent isn't in the remote-auth catalog → blocked
 *   no-methods:     spec exists but declares no methods (e.g. mock) → allowed
 *   files-match:    a non-env method id is listed in remote_credentials → allowed
 *   env-secret:     an env method's secret id is set in remote_auth_secrets → allowed
 *   no-creds:       spec methods exist but neither files nor env are wired → blocked
 */
type CompatReason =
  | "executor-local"
  | "no-spec"
  | "no-methods"
  | "files-match"
  | "env-secret"
  | "no-creds";

function evalAgentCompat(
  agent: { agent_name: string },
  executorProfile: Pick<ExecutorProfile, "config" | "executor_type">,
  authSpecs: RemoteAuthSpec[],
): { ok: boolean; reason: CompatReason } {
  if (!executorRequiresAgentCredentials(executorProfile.executor_type)) {
    return { ok: true, reason: "executor-local" };
  }
  const spec = authSpecs.find((s) => s.id === agent.agent_name);
  if (!spec) return { ok: false, reason: "no-spec" };
  const methods = spec.methods ?? [];
  if (methods.length === 0) return { ok: true, reason: "no-methods" };

  const credentials = new Set(parseJSON<string[]>(executorProfile.config?.remote_credentials, []));
  if (methods.some((m) => m.type !== "env" && credentials.has(m.method_id))) {
    return { ok: true, reason: "files-match" };
  }

  const secrets = parseJSON<Record<string, string | null>>(
    executorProfile.config?.remote_auth_secrets,
    {},
  );
  if (methods.some((m) => m.type === "env" && secrets[m.method_id])) {
    return { ok: true, reason: "env-secret" };
  }
  return { ok: false, reason: "no-creds" };
}

/**
 * For local/worktree executors, agents need no extra credentials → always supported.
 * For remote executors (Docker/Sprites), the executor profile must carry either:
 *   - a non-env auth method ID for the agent's spec in `remote_credentials`, or
 *   - a non-null secret keyed by an env method ID in `remote_auth_secrets`.
 * Agents without a remote-auth spec (Copilot/Amp/TUI) cannot
 * carry credentials on remote executors → blocked. A spec with zero methods
 * means "no credentials needed" (mock-agent for tests) → allowed.
 *
 * Spec IDs are registry-type strings ("claude-acp", "codex-acp", …) which the
 * frontend exposes as `AgentProfileOption.agent_name`. `agent_id` is a DB UUID
 * and is unrelated to the catalog.
 *
 * Per-call decisions are logged to the `[executor-compat]` debug namespace so
 * "agent missing from dialog" reports can be diagnosed by enabling debug logs
 * and reading the reason (no-spec / no-creds / …).
 */
export function isAgentConfiguredOnExecutor(
  agent: { agent_name: string },
  executorProfile: Pick<ExecutorProfile, "config" | "executor_type">,
  authSpecs: RemoteAuthSpec[],
): boolean {
  const result = evalAgentCompat(agent, executorProfile, authSpecs);
  debug("check", {
    agent: agent.agent_name,
    executor_type: executorProfile.executor_type ?? "-",
    spec_count: authSpecs.length,
    ok: result.ok,
    reason: result.reason,
  });
  return result.ok;
}
