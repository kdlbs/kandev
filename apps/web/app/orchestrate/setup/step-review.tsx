"use client";

import { Badge } from "@kandev/ui/badge";
import { Card } from "@kandev/ui/card";

type StepReviewProps = {
  workspaceName: string;
  taskPrefix: string;
  agentName: string;
  agentProfileLabel: string;
  executorPreference: string;
  taskTitle: string;
};

const EXECUTOR_LABELS: Record<string, string> = {
  local_pc: "Local (standalone)",
  local_docker: "Docker",
  sprites: "Sprites (remote)",
};

export function StepReview({
  workspaceName,
  taskPrefix,
  agentName,
  agentProfileLabel,
  executorPreference,
  taskTitle,
}: StepReviewProps) {
  return (
    <div className="space-y-6">
      <div>
        <h2 className="text-xl font-semibold">Review and launch</h2>
        <p className="text-sm text-muted-foreground mt-1">
          Confirm the details below. Everything can be changed later.
        </p>
      </div>
      <Card className="divide-y divide-border">
        <ReviewRow label="Workspace" value={workspaceName || "Default Workspace"}>
          <Badge variant="secondary" className="ml-2">
            {taskPrefix || "KAN"}
          </Badge>
        </ReviewRow>
        <ReviewRow label="CEO Agent" value={agentName || "CEO"}>
          {agentProfileLabel && (
            <span className="text-xs text-muted-foreground ml-2">({agentProfileLabel})</span>
          )}
        </ReviewRow>
        <ReviewRow label="Executor" value={EXECUTOR_LABELS[executorPreference] || "Local"} />
        <ReviewRow label="First task" value={taskTitle || "No initial task"} />
      </Card>
    </div>
  );
}

function ReviewRow({
  label,
  value,
  children,
}: {
  label: string;
  value: string;
  children?: React.ReactNode;
}) {
  return (
    <div className="flex items-center justify-between px-4 py-3">
      <span className="text-sm text-muted-foreground">{label}</span>
      <span className="text-sm font-medium flex items-center">
        {value}
        {children}
      </span>
    </div>
  );
}
