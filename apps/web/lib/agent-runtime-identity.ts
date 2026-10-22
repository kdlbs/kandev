import type { AgentUpdateStatus } from "@/lib/api";
import type { RuntimeUpdate } from "@/lib/types/http";

export type AgentRuntimeIdentity =
  | { kind: "managed"; package: string; version?: string }
  | { kind: "native" | "harness" | "custom" | "unknown" };

export function presentAgentRuntimeIdentity(
  agentName: string,
  runtime?: RuntimeUpdate,
  status?: AgentUpdateStatus,
): AgentRuntimeIdentity {
  if (status) return presentRuntimeStatus(status);
  if (!runtime?.supported) return { kind: "unknown" };
  if (runtime.update_mode === "self_update") return { kind: "harness" };
  // OpenCode can select a native source without marking its npm metadata as a fallback.
  if (agentName === "opencode-acp" || runtime.managed_fallback) return { kind: "unknown" };
  if (runtime.update_mode === "pinned" && runtime.package) {
    return {
      kind: "managed",
      package: runtime.package,
      version: runtime.effective_version || undefined,
    };
  }
  return { kind: "unknown" };
}

function presentRuntimeStatus(status: AgentUpdateStatus): AgentRuntimeIdentity {
  if (status.mechanism === "custom") return { kind: "custom" };
  if (status.update_mode === "self_update") return { kind: "harness" };
  if (
    status.source === "native" ||
    (status.mechanism === "native" && status.owner === "external")
  ) {
    return { kind: "native" };
  }
  if (
    status.owner === "kandev" &&
    status.mechanism === "npm_candidate" &&
    status.management === "managed" &&
    status.package
  ) {
    return {
      kind: "managed",
      package: status.package,
      version: status.effective_version || undefined,
    };
  }
  return { kind: "unknown" };
}
