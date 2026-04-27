"use client";

import { Input } from "@kandev/ui/input";
import { Label } from "@kandev/ui/label";

type StepWorkspaceProps = {
  workspaceName: string;
  taskPrefix: string;
  onChange: (patch: { workspaceName?: string; taskPrefix?: string }) => void;
};

export function StepWorkspace({ workspaceName, taskPrefix, onChange }: StepWorkspaceProps) {
  return (
    <div className="space-y-6">
      <div>
        <h2 className="text-xl font-semibold">Set up your Orchestrate workspace</h2>
        <p className="text-sm text-muted-foreground mt-1">
          Orchestrate manages a team of AI agents that work on your tasks autonomously.
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
            onChange={(e) => onChange({ taskPrefix: e.target.value.toUpperCase() })}
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
