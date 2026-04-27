"use client";

import { Input } from "@kandev/ui/input";
import { Label } from "@kandev/ui/label";
import { Textarea } from "@kandev/ui/textarea";

type StepTaskProps = {
  taskTitle: string;
  taskDescription: string;
  onChange: (patch: { taskTitle?: string; taskDescription?: string }) => void;
};

export function StepTask({ taskTitle, taskDescription, onChange }: StepTaskProps) {
  return (
    <div className="space-y-6">
      <div>
        <h2 className="text-xl font-semibold">Give your CEO something to do</h2>
        <p className="text-sm text-muted-foreground mt-1">
          The CEO will analyze this task, break it into subtasks, and assign them to worker agents.
        </p>
      </div>
      <div className="space-y-4">
        <div>
          <Label htmlFor="task-title">Task title</Label>
          <Input
            id="task-title"
            value={taskTitle}
            onChange={(e) => onChange({ taskTitle: e.target.value })}
            placeholder="Explore the codebase and create an engineering roadmap"
            className="mt-1"
            autoFocus
          />
        </div>
        <div>
          <Label htmlFor="task-desc">Description (optional)</Label>
          <Textarea
            id="task-desc"
            value={taskDescription}
            onChange={(e) => onChange({ taskDescription: e.target.value })}
            placeholder="Provide additional context or requirements for the task..."
            className="mt-1 min-h-[100px]"
          />
        </div>
      </div>
    </div>
  );
}
