"use client";

import { useCallback, useEffect } from "react";
import { useAppStore } from "@/components/state-provider";
import { useOfficeRefetch } from "@/hooks/use-office-refetch";
import { listActivity } from "@/lib/api/domains/office-api";
import type { ActivityEntry } from "@/lib/state/slices/office/types";
import { ActivityFeed } from "./activity-feed";

type ActivityPageClientProps = {
  initialActivity: ActivityEntry[];
};

export function ActivityPageClient({ initialActivity }: ActivityPageClientProps) {
  const activeWorkspaceId = useAppStore((s) => s.workspaces.activeId);
  const setActivity = useAppStore((s) => s.setActivity);

  useEffect(() => {
    if (initialActivity.length > 0) {
      setActivity(initialActivity);
    }
  }, [initialActivity, setActivity]);

  const refetchActivity = useCallback(async () => {
    if (!activeWorkspaceId) return;
    const res = await listActivity(activeWorkspaceId).catch(() => ({
      activity: [] as ActivityEntry[],
    }));
    setActivity(res.activity ?? []);
  }, [activeWorkspaceId, setActivity]);

  useOfficeRefetch("activity", refetchActivity);

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
