import { t } from "@/lib/i18n";
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

// Catalog keys, resolved per call: this module is imported once, so a `t()`
// here would freeze at the boot locale (docs/i18n.md).
const workspaceMethodKeys: Record<string, string> = {
  pat: "task:credentialWorkspacePat",
  gh_cli: "task:credentialWorkspaceGhCli",
  github_app_installation: "task:credentialWorkspaceGithubApp",
};

const transportKeys: Record<string, string> = {
  managed_https: "task:credentialTransportManagedHttps",
  profile_token: "task:credentialTransportProfileToken",
  executor_selected: "task:credentialTransportExecutorSelected",
};

function label(keys: Record<string, string>, id: string, fallbackKey: string): string {
  return t(keys[id] ?? fallbackKey);
}

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
        source: t("task:credentialSourceWorkspace"),
        detail: label(workspaceMethodKeys, workspaceMethod, "task:credentialWorkspaceFallback"),
        transport: label(transportKeys, transport, "task:credentialTransportManagedFallback"),
      };
    case "executor_profile":
      return {
        source: t("task:credentialSourceExecutorProfile"),
        detail: actor || t("task:credentialExplicitToken"),
        transport: label(transportKeys, transport, "task:credentialTransportProfileToken"),
      };
    case "executor":
      return {
        source: t("task:credentialSourceExecutor"),
        detail: actor || t("task:credentialsAvailableInExecutor"),
        transport: label(transportKeys, transport, "task:credentialTransportExecutorSelected"),
      };
    default:
      return null;
  }
}
