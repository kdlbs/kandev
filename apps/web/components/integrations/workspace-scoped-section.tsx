"use client";

import { type ReactNode } from "react";
import { Card, CardContent } from "@kandev/ui/card";
import { useAppStore } from "@/components/state-provider";
import { useWorkspaces } from "@/hooks/domains/workspace/use-workspaces";

type WorkspaceScopedSectionProps = {
  emptyMessage?: string;
  workspaceId?: string;
  children: (workspaceId: string) => ReactNode;
};

// WorkspaceScopedSection renders per-workspace integration settings for the
// routed workspace when present, otherwise the workspace currently selected
// in the top-right settings switcher.
export function WorkspaceScopedSection({
  emptyMessage,
  workspaceId,
  children,
}: WorkspaceScopedSectionProps) {
  const { items: workspaces } = useWorkspaces();
  const activeId = useAppStore((s) => s.workspaces.activeId);
  const selected = workspaceId ?? activeId ?? workspaces[0]?.id ?? null;

  if (!workspaceId && workspaces.length === 0) {
    return (
      <Card>
        <CardContent className="py-6 text-sm text-muted-foreground">
          {emptyMessage ?? "Create a workspace to configure this integration."}
        </CardContent>
      </Card>
    );
  }

  return <div className="space-y-3">{selected ? children(selected) : null}</div>;
}
