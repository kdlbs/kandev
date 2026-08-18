import { linkToOfficeHome, linkToTaskOverview } from "@/lib/links";
import { isOfficeWorkspace, type ModeWorkspace } from "@/lib/state/slices/workspace/selectors";

/** Returns the home destination for a workspace without depending on UI code. */
export function workspaceHomeHref(workspace: ModeWorkspace | undefined): string {
  if (!workspace) return linkToTaskOverview();
  if (!isOfficeWorkspace(workspace)) return linkToTaskOverview({ workspaceId: workspace.id });
  return linkToOfficeHome({ workspaceId: workspace.id });
}
