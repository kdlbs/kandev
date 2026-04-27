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

export function CreateRoutineDialog({
  open,
  onOpenChange,
  agents,
  onSubmit,
}: CreateRoutineDialogProps) {
  const [name, setName] = useState("");
  const [description, setDescription] = useState("");
  const [taskTitle, setTaskTitle] = useState("");
  const [taskDesc, setTaskDesc] = useState("");
  const [assignee, setAssignee] = useState("");
  const [concurrency, setConcurrency] = useState("coalesce_if_active");
  const [triggerKind, setTriggerKind] = useState("cron");
  const [cronExpr, setCronExpr] = useState("");
  const [timezone, setTimezone] = useState("UTC");

  function handleSubmit() {
    onSubmit({
      name,
      description,
      taskTitle,
      taskDescription: taskDesc,
      assigneeAgentInstanceId: assignee,
      concurrencyPolicy: concurrency,
      triggerKind,
      cronExpression: cronExpr,
      timezone,
    });
    resetForm();
  }

  function resetForm() {
    setName("");
    setDescription("");
    setTaskTitle("");
    setTaskDesc("");
    setAssignee("");
    setConcurrency("coalesce_if_active");
    setTriggerKind("cron");
    setCronExpr("");
    setTimezone("UTC");
  }

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="max-w-lg">
        <DialogHeader>
          <DialogTitle>Create Routine</DialogTitle>
        </DialogHeader>
        <div className="space-y-4">
          <div>
            <Label>Name</Label>
            <Input
              value={name}
              onChange={(e) => setName(e.target.value)}
              placeholder="Daily Dep Update"
            />
          </div>
          <div>
            <Label>Description</Label>
            <Textarea
              value={description}
              onChange={(e) => setDescription(e.target.value)}
              rows={2}
            />
          </div>
          <div>
            <Label>Task Title Template</Label>
            <Input
              value={taskTitle}
              onChange={(e) => setTaskTitle(e.target.value)}
              placeholder="{{name}} - {{date}}"
            />
            <p className="text-xs text-muted-foreground mt-1">
              Title for auto-created tasks. Use &#123;&#123;name&#125;&#125; and
              &#123;&#123;date&#125;&#125; as placeholders.
            </p>
          </div>
          <div>
            <Label>Task Description Template</Label>
            <Textarea value={taskDesc} onChange={(e) => setTaskDesc(e.target.value)} rows={2} />
            <p className="text-xs text-muted-foreground mt-1">
              Instructions the agent receives when this routine triggers
            </p>
          </div>
          <div className="grid grid-cols-2 gap-4">
            <div>
              <Label>Assignee</Label>
              <Select value={assignee} onValueChange={setAssignee}>
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
              <Select value={concurrency} onValueChange={setConcurrency}>
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
          <div className="grid grid-cols-2 gap-4">
            <div>
              <Label>Trigger Type</Label>
              <Select value={triggerKind} onValueChange={setTriggerKind}>
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
                  onChange={(e) => setCronExpr(e.target.value)}
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
                onChange={(e) => setTimezone(e.target.value)}
                placeholder="UTC"
              />
            </div>
          )}
        </div>
        <DialogFooter>
          <Button variant="ghost" onClick={() => onOpenChange(false)} className="cursor-pointer">
            Cancel
          </Button>
          <Button onClick={handleSubmit} disabled={!name} className="cursor-pointer">
            Create
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
