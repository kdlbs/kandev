"use client";

import { WorkspaceEditClient } from "@/app/settings/workspace/workspace-edit-client";

export default function WorkspaceEditPage({ workspaceId }: { workspaceId: string }) {
  return <WorkspaceEditClient workspaceId={workspaceId} />;
}
