"use client";

import { useState } from "react";
import { Button } from "@kandev/ui/button";
import { Input } from "@kandev/ui/input";
import { Label } from "@kandev/ui/label";
import { Textarea } from "@kandev/ui/textarea";
import { Dialog, DialogContent, DialogHeader, DialogTitle, DialogFooter } from "@kandev/ui/dialog";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@kandev/ui/select";
import type { AgentInstance } from "@/lib/state/slices/orchestrate/types";

type CreateRoutineDialogProps = {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  agents: AgentInstance[];
  onSubmit: (data: {
    name: string;
    description: string;
    taskTitle: string;
    taskDescription: string;
    assigneeAgentInstanceId: string;
    concurrencyPolicy: string;
    triggerKind: string;
    cronExpression: string;
    timezone: string;
  }) => void;
};

function AssigneeAndConcurrency({
  assignee,
  concurrency,
  agents,
  onAssigneeChange,
  onConcurrencyChange,
}: {
  assignee: string;
  concurrency: string;
  agents: AgentInstance[];
  onAssigneeChange: (v: string) => void;
  onConcurrencyChange: (v: string) => void;
}) {
  return (
    <div className="grid grid-cols-2 gap-4">
      <div>
        <Label>Assignee</Label>
        <Select value={assignee} onValueChange={onAssigneeChange}>
          <SelectTrigger className="cursor-pointer">
            <SelectValue placeholder="Select agent" />
          </SelectTrigger>
          <SelectContent>
            {agents.map((a) => (
              <SelectItem key={a.id} value={a.id} className="cursor-pointer">
                {a.name}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>
      </div>
      <div>
        <Label>Concurrency</Label>
        <Select value={concurrency} onValueChange={onConcurrencyChange}>
          <SelectTrigger className="cursor-pointer">
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            <SelectItem value="skip_if_active" className="cursor-pointer">
              Skip if active
            </SelectItem>
            <SelectItem value="coalesce_if_active" className="cursor-pointer">
              Coalesce
            </SelectItem>
            <SelectItem value="always_create" className="cursor-pointer">
              Always create
            </SelectItem>
          </SelectContent>
        </Select>
        <p className="text-xs text-muted-foreground mt-1">
          What happens if the previous run is still active
        </p>
      </div>
    </div>
  );
}

function TriggerSection({
  triggerKind,
  cronExpr,
  timezone,
  onTriggerKindChange,
  onCronExprChange,
  onTimezoneChange,
}: {
  triggerKind: string;
  cronExpr: string;
  timezone: string;
  onTriggerKindChange: (v: string) => void;
  onCronExprChange: (v: string) => void;
  onTimezoneChange: (v: string) => void;
}) {
  return (
    <>
      <div className="grid grid-cols-2 gap-4">
        <div>
          <Label>Trigger Type</Label>
          <Select value={triggerKind} onValueChange={onTriggerKindChange}>
            <SelectTrigger className="cursor-pointer">
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value="cron" className="cursor-pointer">
                Cron
              </SelectItem>
              <SelectItem value="webhook" className="cursor-pointer">
                Webhook
              </SelectItem>
              <SelectItem value="manual" className="cursor-pointer">
                Manual
              </SelectItem>
            </SelectContent>
          </Select>
        </div>
        {triggerKind === "cron" && (
          <div>
            <Label>Cron Expression</Label>
            <Input
              value={cronExpr}
              onChange={(e) => onCronExprChange(e.target.value)}
              placeholder="0 9 * * *"
            />
            <p className="text-xs text-muted-foreground mt-1">
              Standard cron expression (e.g. 0 9 * * MON for every Monday at 9am)
            </p>
          </div>
        )}
      </div>
      {triggerKind === "cron" && (
        <div>
          <Label>Timezone</Label>
          <Input
            value={timezone}
            onChange={(e) => onTimezoneChange(e.target.value)}
            placeholder="UTC"
          />
        </div>
      )}
    </>
  );
}

type RoutineFormState = {
  name: string;
  description: string;
  taskTitle: string;
  taskDesc: string;
  assignee: string;
  concurrency: string;
  triggerKind: string;
  cronExpr: string;
  timezone: string;
};

const INITIAL_ROUTINE_STATE: RoutineFormState = {
  name: "",
  description: "",
  taskTitle: "",
  taskDesc: "",
  assignee: "",
  concurrency: "coalesce_if_active",
  triggerKind: "cron",
  cronExpr: "",
  timezone: "UTC",
};

function BasicFields({
  state,
  onUpdate,
}: {
  state: RoutineFormState;
  onUpdate: (patch: Partial<RoutineFormState>) => void;
}) {
  return (
    <>
      <div>
        <Label>Name</Label>
        <Input
          value={state.name}
          onChange={(e) => onUpdate({ name: e.target.value })}
          placeholder="Daily Dep Update"
        />
      </div>
      <div>
        <Label>Description</Label>
        <Textarea
          value={state.description}
          onChange={(e) => onUpdate({ description: e.target.value })}
          rows={2}
        />
      </div>
      <div>
        <Label>Task Title Template</Label>
        <Input
          value={state.taskTitle}
          onChange={(e) => onUpdate({ taskTitle: e.target.value })}
          placeholder="{{name}} - {{date}}"
        />
        <p className="text-xs text-muted-foreground mt-1">
          Title for auto-created tasks. Use &#123;&#123;name&#125;&#125; and
          &#123;&#123;date&#125;&#125; as placeholders.
        </p>
      </div>
      <div>
        <Label>Task Description Template</Label>
        <Textarea
          value={state.taskDesc}
          onChange={(e) => onUpdate({ taskDesc: e.target.value })}
          rows={2}
        />
        <p className="text-xs text-muted-foreground mt-1">
          Instructions the agent receives when this routine triggers
        </p>
      </div>
    </>
  );
}

export function CreateRoutineDialog({
  open,
  onOpenChange,
  agents,
  onSubmit,
}: CreateRoutineDialogProps) {
  const [state, setState] = useState<RoutineFormState>(INITIAL_ROUTINE_STATE);
  const update = (patch: Partial<RoutineFormState>) => setState((prev) => ({ ...prev, ...patch }));

  function handleSubmit() {
    onSubmit({
      name: state.name,
      description: state.description,
      taskTitle: state.taskTitle,
      taskDescription: state.taskDesc,
      assigneeAgentInstanceId: state.assignee,
      concurrencyPolicy: state.concurrency,
      triggerKind: state.triggerKind,
      cronExpression: state.cronExpr,
      timezone: state.timezone,
    });
    setState(INITIAL_ROUTINE_STATE);
  }

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="max-w-lg">
        <DialogHeader>
          <DialogTitle>Create Routine</DialogTitle>
        </DialogHeader>
        <div className="space-y-4">
          <BasicFields state={state} onUpdate={update} />
          <AssigneeAndConcurrency
            assignee={state.assignee}
            concurrency={state.concurrency}
            agents={agents}
            onAssigneeChange={(v) => update({ assignee: v })}
            onConcurrencyChange={(v) => update({ concurrency: v })}
          />
          <TriggerSection
            triggerKind={state.triggerKind}
            cronExpr={state.cronExpr}
            timezone={state.timezone}
            onTriggerKindChange={(v) => update({ triggerKind: v })}
            onCronExprChange={(v) => update({ cronExpr: v })}
            onTimezoneChange={(v) => update({ timezone: v })}
          />
        </div>
        <DialogFooter>
          <Button variant="ghost" onClick={() => onOpenChange(false)} className="cursor-pointer">
            Cancel
          </Button>
          <Button onClick={handleSubmit} disabled={!state.name} className="cursor-pointer">
            Create
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
