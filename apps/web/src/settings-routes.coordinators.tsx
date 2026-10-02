import type { ReactNode } from "react";

import CoordinatorsPage from "@/app/settings/workspace/[id]/coordinators/page";
import CoordinatorEditorRoutePage from "@/app/settings/workspace/[id]/coordinators/[coordinatorId]/page";
import NewCoordinatorPage from "@/app/settings/workspace/[id]/coordinators/new/page";
import { useFeature } from "@/hooks/domains/features/use-feature";
import { WorkspaceSettingsShell } from "@/components/settings/workspaces/workspace-settings-shell";
import { SettingsRouteFallback } from "./settings-route-helpers";

/**
 * Coordinators workspace-settings routing. Lives here rather than in
 * `settings-routes.tsx` to keep that file inside its line budget.
 */

function renderCoordinatorRoutePage(workspaceId: string, coordinatorId: string | null): ReactNode {
  if (coordinatorId === null) return <CoordinatorsPage workspaceId={workspaceId} />;
  if (coordinatorId === "new") return <NewCoordinatorPage workspaceId={workspaceId} />;
  return <CoordinatorEditorRoutePage workspaceId={workspaceId} coordinatorId={coordinatorId} />;
}

// Coordinators is flag-gated (unlike automations), so the flag can only be
// read inside a real component: `renderWorkspaceSettingsRoute` and its peers
// are plain functions invoked synchronously during another component's
// render, and calling a hook directly from one of them would make that
// component's hook order depend on which pathname it happened to receive.
function WorkspaceCoordinatorRoute({
  workspaceId,
  coordinatorId,
}: {
  workspaceId: string;
  coordinatorId: string | null;
}): ReactNode {
  const coordinatorEnabled = useFeature("coordinator");
  if (!coordinatorEnabled) {
    const pathname = coordinatorId
      ? `/settings/workspaces/${workspaceId}/coordinators/${coordinatorId}`
      : `/settings/workspaces/${workspaceId}/coordinators`;
    return <SettingsRouteFallback pathname={pathname} />;
  }
  return (
    <WorkspaceSettingsShell workspaceId={workspaceId} activeTab="coordinators">
      {renderCoordinatorRoutePage(workspaceId, coordinatorId)}
    </WorkspaceSettingsShell>
  );
}

export function renderWorkspaceCoordinatorRoute(
  workspaceId: string,
  coordinatorId: string | null,
): ReactNode {
  return <WorkspaceCoordinatorRoute workspaceId={workspaceId} coordinatorId={coordinatorId} />;
}
