"use client";

import { useAppStore } from "@/components/state-provider";
import { ActivityFeed } from "./activity-feed";

export default function ActivityPage() {
  const activeWorkspaceId = useAppStore((s) => s.workspaces.activeId);

  if (!activeWorkspaceId) {
    return (
      <div className="p-6">
        <p className="text-sm text-muted-foreground">Select a workspace to view activity.</p>
      </div>
    );
  }

  return (
    <div className="p-6">
      <ActivityFeed workspaceId={activeWorkspaceId} />
    </div>
  );
}
