"use client";

import { Input } from "@kandev/ui/input";
import { Label } from "@kandev/ui/label";
import { Textarea } from "@kandev/ui/textarea";
import {
  DEFAULT_ONBOARDING_TASK_DESCRIPTION,
  DEFAULT_ONBOARDING_TASK_TITLE,
} from "./setup-task-defaults";

type StepTaskProps = {
  agentName: string;
  taskTitle: string;
  taskDescription: string;
  onChange: (patch: { taskTitle?: string; taskDescription?: string }) => void;
};

export function StepTask({ agentName, taskTitle, taskDescription, onChange }: StepTaskProps) {
  // Step 1 (agent creation) requires a non-empty agentName before advancing,
  // so by the time this step renders we always have a real value.
  const name = agentName.trim() || "coordinator";
  return (
    <div className="space-y-6">
      <div>
        <h2 className="text-xl font-semibold">Give your {name} something to do</h2>
        <p className="text-sm text-muted-foreground mt-1">
          {name} will use this starter task to set up projects, agents, and the initial backlog.
        </p>
      </div>
      <div className="space-y-4">
        <div>
          <Label htmlFor="task-title">Task title</Label>
          <Input
            id="task-title"
            value={taskTitle}
            onChange={(e) => onChange({ taskTitle: e.target.value })}
            placeholder={DEFAULT_ONBOARDING_TASK_TITLE}
            className="mt-1"
            autoFocus
          />
        </div>
        <div>
          <Label htmlFor="task-desc">Description</Label>
          <Textarea
            id="task-desc"
            value={taskDescription}
            onChange={(e) => onChange({ taskDescription: e.target.value })}
            placeholder={DEFAULT_ONBOARDING_TASK_DESCRIPTION}
            className="mt-1 min-h-[220px]"
          />
        </div>
      </div>
    </div>
  );
}
