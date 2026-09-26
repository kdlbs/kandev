import type { AgentDiscovery, ModelDiscovery } from "@/lib/types/http";

export type HostCLIVersionPresentation =
  | { kind: "known"; version: string }
  | { kind: "unknown"; reason: string };

/**
 * Reduces an agent's discovered CLI version state to what the Agents
 * settings card renders. Returns null for an agent type with no host CLI
 * (both fields absent) so the card shows nothing extra.
 */
export function presentHostCLIVersion(
  agent: Pick<AgentDiscovery, "cli_version" | "cli_version_error">,
): HostCLIVersionPresentation | null {
  if (agent.cli_version) return { kind: "known", version: agent.cli_version };
  if (agent.cli_version_error) return { kind: "unknown", reason: agent.cli_version_error };
  return null;
}

export type ModelDiscoveryPresentation =
  | { kind: "cli"; executable: string; version?: string }
  | { kind: "no_listing"; executable: string }
  | { kind: "failed"; reason: string };

/**
 * Reduces a model-discovery block to what the note under the profile model
 * selector renders. Returns null while nothing has been discovered yet
 * ("pending") or for an agent type without a host CLI, so the note stays
 * absent instead of flickering through a transient state.
 */
export function presentModelDiscovery(
  discovery: ModelDiscovery | undefined,
): ModelDiscoveryPresentation | null {
  if (!discovery || discovery.status === "pending") return null;
  if (discovery.status === "ok" && discovery.source === "cli_command") {
    return { kind: "cli", executable: discovery.executable ?? "", version: discovery.cli_version };
  }
  if (discovery.status === "ok" || discovery.status === "skipped") {
    return { kind: "no_listing", executable: discovery.executable ?? "" };
  }
  return { kind: "failed", reason: discovery.error || discovery.status };
}
