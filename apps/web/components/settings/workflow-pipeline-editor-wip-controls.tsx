"use client";

import { Input } from "@kandev/ui/input";
import { Label } from "@kandev/ui/label";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@kandev/ui/select";
import type { WorkflowStep } from "@/lib/types/http";
import { HelpTip } from "./workflow-pipeline-editor-helpers";

type StepWipControlsProps = {
  step: WorkflowStep;
  steps: WorkflowStep[];
  onUpdate: (updates: Partial<WorkflowStep>) => void;
  readOnly: boolean;
};

function parseWipLimit(value: string): number {
  const next = Number.parseInt(value, 10);
  if (!Number.isFinite(next)) return 0;
  return Math.max(0, next);
}

export function StepWipControls({ step, steps, onUpdate, readOnly }: StepWipControlsProps) {
  const otherSteps = steps.filter((s) => s.id !== step.id);
  const pullFromValue = step.pull_from_step_id || "none";
  const pullFromSelectID = `${step.id}-pull-from-step`;

  return (
    <div className="grid grid-cols-1 gap-3 pt-3 sm:grid-cols-2 xl:grid-cols-[180px_minmax(220px,320px)]">
      <div className="space-y-1.5">
        <div className="flex items-center gap-1.5">
          <Label htmlFor={`${step.id}-wip-limit`} className="text-xs font-medium">
            WIP limit
          </Label>
          <HelpTip text="Maximum tasks allowed in this step at once. Use 0 for unlimited." />
        </div>
        <Input
          id={`${step.id}-wip-limit`}
          data-testid={`${step.id}-wip-limit-input`}
          type="number"
          min={0}
          inputMode="numeric"
          value={step.wip_limit ?? 0}
          onChange={(e) => {
            if (readOnly) return;
            onUpdate({ wip_limit: parseWipLimit(e.target.value) });
          }}
          disabled={readOnly}
          className="h-8"
        />
      </div>
      <div className="space-y-1.5">
        <div className="flex items-center gap-1.5">
          <Label htmlFor={pullFromSelectID} className="text-xs font-medium">
            Pull from
          </Label>
          <HelpTip text="Optional feeder step to pull work from when this step has capacity." />
        </div>
        <Select
          value={pullFromValue}
          onValueChange={(value) => {
            if (readOnly) return;
            onUpdate({ pull_from_step_id: value === "none" ? "" : value });
          }}
          disabled={readOnly || otherSteps.length === 0}
        >
          <SelectTrigger
            id={pullFromSelectID}
            className="h-8"
            data-testid={`${step.id}-pull-from-step-select`}
          >
            <SelectValue placeholder="No feeder step" />
          </SelectTrigger>
          <SelectContent>
            <SelectItem value="none" className="cursor-pointer">
              No feeder step
            </SelectItem>
            {otherSteps.map((candidate) => (
              <SelectItem key={candidate.id} value={candidate.id} className="cursor-pointer">
                {candidate.name}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>
      </div>
    </div>
  );
}
