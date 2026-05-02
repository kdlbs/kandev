"use client";

import { useState, type ReactNode } from "react";
import { Card, CardContent } from "@kandev/ui/card";
import { useAppStore } from "@/components/state-provider";
import { WorkspaceSwitcher } from "@/components/task/workspace-switcher";

type WorkspaceScopedSectionProps = {
  label?: string;
  emptyMessage?: string;
  children: (workspaceId: string) => ReactNode;
};

// WorkspaceScopedSection wraps watcher / per-workspace settings under a
// workspace selector so the install-wide integration settings page can still
// surface things that genuinely scope to one workspace at a time. The selector
// defaults to the active workspace from the global store; an explicit user
// override survives until the workspace list no longer contains it.
export function WorkspaceScopedSection({
  label = "Workspace",
  emptyMessage,
  children,
}: WorkspaceScopedSectionProps) {
  const workspaces = useAppStore((s) => s.workspaces.items);
  const activeId = useAppStore((s) => s.workspaces.activeId);
  const [override, setOverride] = useState<string | null>(null);

  // Derive the effective selection at render time: honour an in-bounds user
  // override, otherwise fall back to the active workspace (or the first one).
  // This avoids setState-in-effect when the workspace list hydrates after
  // first render.
  const overrideValid = override && workspaces.some((w) => w.id === override);
  const selected = overrideValid ? override : (activeId ?? workspaces[0]?.id ?? null);

  if (workspaces.length === 0) {
    return (
      <Card>
        <CardContent className="py-6 text-sm text-muted-foreground">
          {emptyMessage ?? "Create a workspace to configure this integration."}
        </CardContent>
      </Card>
    );
  }

  return (
    <div className="space-y-3">
      <div className="flex items-center gap-3 text-sm">
        <span className="text-muted-foreground">{label}</span>
        <WorkspaceSwitcher
          workspaces={workspaces}
          activeWorkspaceId={selected}
          onSelect={setOverride}
        />
      </div>
      {selected ? children(selected) : null}
    </div>
  );
}
