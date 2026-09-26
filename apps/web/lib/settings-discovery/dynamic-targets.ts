export type WorkspaceDiscoveryTarget = "name" | "default-executor" | "default-agent-profile";

export type AgentProfileDiscoveryTarget =
  | "profile-settings"
  | "cli-flags"
  | "environment-variables"
  | "command-preview";

export type ExecutorProfileDiscoveryTarget =
  | "profile-details"
  | "environment-variables"
  | "prepare-script"
  | "mcp-policy";

export function workspaceDiscoveryTarget(
  workspaceId: string,
  target: WorkspaceDiscoveryTarget,
): string {
  return `setting-workspace-${workspaceId}-${target}`;
}

export function agentProfileDiscoveryTarget(
  profileId: string,
  target: AgentProfileDiscoveryTarget,
): string {
  return `setting-agent-profile-${profileId}-${target}`;
}

export function executorProfileDiscoveryTarget(
  profileId: string,
  target: ExecutorProfileDiscoveryTarget,
): string {
  return `setting-executor-profile-${profileId}-${target}`;
}
