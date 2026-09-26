import { linkToOfficeHome, linkToTaskOverview, linkToThreads } from "@/lib/links";
import { isOfficeWorkspace, type ModeWorkspace } from "@/lib/state/slices/workspace/selectors";
import type { StartupPage } from "@/lib/types/http-user-settings";

export function resolveHomeHref({
  workspaceId,
  inOffice,
  startupPage = "task_overview",
}: {
  workspaceId: string | null;
  inOffice: boolean;
  startupPage?: StartupPage;
}): string {
  if (inOffice) return linkToOfficeHome({ workspaceId: workspaceId ?? undefined });
  if (workspaceId && startupPage === "threads") return linkToThreads(workspaceId);
  return linkToTaskOverview({ workspaceId: workspaceId ?? undefined });
}

/** Returns the home destination for a workspace without depending on UI code. */
export function workspaceHomeHref(
  workspace: ModeWorkspace | undefined,
  startupPage?: StartupPage,
): string {
  return resolveHomeHref({
    workspaceId: workspace?.id ?? null,
    inOffice: isOfficeWorkspace(workspace),
    startupPage,
  });
}
