"use client";

import { type ReactNode } from "react";
import { Card, CardContent } from "@kandev/ui/card";
import { useAppStore } from "@/components/state-provider";

type WorkspaceScopedSectionProps = {
  emptyMessage?: string;
  children: (workspaceId: string) => ReactNode;
};

// WorkspaceScopedSection renders per-workspace integration settings for the
// workspace currently selected in the top-right settings switcher. The switcher
// (settings-layout-client) is the single source of truth for which workspace is
// being edited, so this component just reads the active workspace from the
// store — it no longer renders its own selector.
export function WorkspaceScopedSection({ emptyMessage, children }: WorkspaceScopedSectionProps) {
  const workspaces = useAppStore((s) => s.workspaces.items);
  const activeId = useAppStore((s) => s.workspaces.activeId);
  const selected = activeId ?? workspaces[0]?.id ?? null;

  if (workspaces.length === 0) {
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
