import type { ExecutorProfile } from "@/lib/types/http";
import type { RemoteAuthSpec } from "@/lib/api/domains/settings-api";

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
 * For local/worktree executors, agents need no extra credentials → always supported.
 * For remote executors (Docker/Sprites), the executor profile must carry either:
 *   - a non-env auth method ID for the agent's spec in `remote_credentials`, or
 *   - a non-null secret keyed by an env method ID in `remote_auth_secrets`.
 * Agents without a remote-auth spec (Copilot/Amp/OpenCode/TUI) cannot
 * carry credentials on remote executors → blocked. A spec with zero methods
 * means "no credentials needed" (mock-agent for tests) → allowed.
 *
 * Spec IDs are registry-type strings ("claude-acp", "codex-acp", …) which the
 * frontend exposes as `AgentProfileOption.agent_name`. `agent_id` is a DB UUID
 * and is unrelated to the catalog.
 */
export function isAgentConfiguredOnExecutor(
  agent: { agent_name: string },
  executorProfile: Pick<ExecutorProfile, "config" | "executor_type">,
  authSpecs: RemoteAuthSpec[],
): boolean {
  if (!executorRequiresAgentCredentials(executorProfile.executor_type)) return true;

  const spec = authSpecs.find((s) => s.id === agent.agent_name);
  if (!spec) return false;
  if (spec.methods.length === 0) return true;

  const credentials = new Set(parseJSON<string[]>(executorProfile.config?.remote_credentials, []));
  if (spec.methods.some((m) => m.type !== "env" && credentials.has(m.method_id))) return true;

  const secrets = parseJSON<Record<string, string | null>>(
    executorProfile.config?.remote_auth_secrets,
    {},
  );
  return spec.methods.some((m) => m.type === "env" && secrets[m.method_id]);
}
