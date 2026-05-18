"use client";

import { useEffect, useState } from "react";
import { Input } from "@kandev/ui/input";
import { Label } from "@kandev/ui/label";

type StepWorkspaceProps = {
  workspaceName: string;
  taskPrefix: string;
  onChange: (patch: { workspaceName?: string; taskPrefix?: string }) => void;
};

export function derivePrefix(name: string): string {
  const cleaned = name.replace(/[^a-zA-Z0-9]/g, "").toUpperCase();
  return cleaned.slice(0, 3) || "KAN";
}

export function StepWorkspace({ workspaceName, taskPrefix, onChange }: StepWorkspaceProps) {
  const [prefixDirty, setPrefixDirty] = useState(false);

  useEffect(() => {
    if (prefixDirty) return;
    const derived = derivePrefix(workspaceName);
    if (derived !== taskPrefix) onChange({ taskPrefix: derived });
    // onChange/taskPrefix intentionally omitted: we only resync when workspace name or
    // dirty flag changes, otherwise this would fight with the parent's state updates.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [workspaceName, prefixDirty]);

  return (
    <div className="space-y-6">
      <div>
        <h2 className="text-xl font-semibold">Set up your Office workspace</h2>
        <p className="text-sm text-muted-foreground mt-1">
          Office turns your backlog into autonomous work. A coordinator agent breaks each task down,
          delegates to specialized worker agents, and reports progress back to you. In the next
          steps you&apos;ll name this workspace, create your coordinator, and optionally hand it a
          first task to kick things off.
        </p>
      </div>
      <div className="space-y-4">
        <div>
          <Label htmlFor="workspace-name">Workspace name</Label>
          <Input
            id="workspace-name"
            value={workspaceName}
            onChange={(e) => onChange({ workspaceName: e.target.value })}
            placeholder="Default Workspace"
            className="mt-1"
            autoFocus
          />
          <p className="text-xs text-muted-foreground mt-1">
            A name for your workspace. You can change this later.
          </p>
        </div>
        <div>
          <Label htmlFor="task-prefix">Task prefix</Label>
          <Input
            id="task-prefix"
            value={taskPrefix}
            onChange={(e) => {
              setPrefixDirty(true);
              onChange({ taskPrefix: e.target.value.toUpperCase() });
            }}
            placeholder="KAN"
            className="mt-1 max-w-32"
            maxLength={6}
          />
          <p className="text-xs text-muted-foreground mt-1">
            Tasks will be numbered {taskPrefix || "KAN"}-1, {taskPrefix || "KAN"}-2, etc.
          </p>
        </div>
      </div>
    </div>
  );
}
