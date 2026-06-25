"use client";

import { useAppStore } from "@/components/state-provider";
import type { ActivityEntry } from "@/lib/state/slices/office/types";
import { ActivityFeed } from "./activity-feed";

type ActivityPageClientProps = {
  initialActivity: ActivityEntry[];
};

export function ActivityPageClient({ initialActivity }: ActivityPageClientProps) {
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
      <ActivityFeed workspaceId={activeWorkspaceId} initialActivity={initialActivity} />
    </div>
  );
}
