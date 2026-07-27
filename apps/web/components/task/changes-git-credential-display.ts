export type GitCredentialDisplay = {
  source: string;
  detail: string;
  transport: string;
};

type GitCredentialSnapshot = {
  source?: unknown;
  workspace_method?: unknown;
  actor?: unknown;
  transport?: unknown;
};

const workspaceMethodLabels: Record<string, string> = {
  pat: "workspace personal access token",
  gh_cli: "workspace GitHub CLI account",
  github_app_installation: "workspace GitHub App",
};

const transportLabels: Record<string, string> = {
  managed_https: "Kandev-managed GitHub HTTPS and gh",
  profile_token: "executor-profile token",
  executor_selected: "executor Git and gh credentials",
};

function readSnapshot(
  metadata: Record<string, unknown> | null | undefined,
): GitCredentialSnapshot | null {
  const snapshot = metadata?.git_credential_snapshot;
  if (!snapshot || typeof snapshot !== "object") return null;
  return snapshot as GitCredentialSnapshot;
}

/** Returns safe, launch-time credential routing copy for the Changes panel. */
export function getGitCredentialDisplay(
  metadata: Record<string, unknown> | null | undefined,
): GitCredentialDisplay | null {
  const snapshot = readSnapshot(metadata);
  if (!snapshot || typeof snapshot.source !== "string") return null;
  const actor = typeof snapshot.actor === "string" ? snapshot.actor : "";
  const workspaceMethod =
    typeof snapshot.workspace_method === "string" ? snapshot.workspace_method : "";
  const transport = typeof snapshot.transport === "string" ? snapshot.transport : "";
  switch (snapshot.source) {
    case "workspace":
      return {
        source: "Managed workspace identity",
        detail: workspaceMethodLabels[workspaceMethod] ?? "workspace GitHub credentials",
        transport: transportLabels[transport] ?? "Kandev-managed task credentials",
      };
    case "executor_profile":
      return {
        source: "Executor-profile token",
        detail: actor || "An explicit GH_TOKEN or GITHUB_TOKEN",
        transport: transportLabels[transport] ?? "executor-profile token",
      };
    case "executor":
      return {
        source: "Executor credentials",
        detail: actor || "Credentials available in the selected executor",
        transport: transportLabels[transport] ?? "executor Git and gh credentials",
      };
    default:
      return null;
  }
}
