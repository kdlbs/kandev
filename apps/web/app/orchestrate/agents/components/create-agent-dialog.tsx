"use client";

import { useState, useCallback } from "react";
import { Button } from "@kandev/ui/button";
import { Input } from "@kandev/ui/input";
import { Label } from "@kandev/ui/label";
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogFooter,
} from "@kandev/ui/dialog";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@kandev/ui/select";
import { useAppStore } from "@/components/state-provider";
import { createAgentInstance } from "@/lib/api/domains/orchestrate-api";
import type { AgentRole, AgentInstance } from "@/lib/state/slices/orchestrate/types";

type CreateAgentDialogProps = {
  open: boolean;
  onOpenChange: (open: boolean) => void;
};

export function CreateAgentDialog({ open, onOpenChange }: CreateAgentDialogProps) {
  const workspaceId = useAppStore((s) => s.workspaces.activeId);
  const agents = useAppStore((s) => s.orchestrate.agentInstances);
  const addAgentInstance = useAppStore((s) => s.addAgentInstance);

  const [name, setName] = useState("");
  const [role, setRole] = useState<AgentRole>("worker");
  const [reportsTo, setReportsTo] = useState("");
  const [budgetCents, setBudgetCents] = useState(0);
  const [maxConcurrent, setMaxConcurrent] = useState(1);
  const [executorPref, setExecutorPref] = useState("");
  const [submitting, setSubmitting] = useState(false);

  const resetForm = useCallback(() => {
    setName("");
    setRole("worker");
    setReportsTo("");
    setBudgetCents(0);
    setMaxConcurrent(1);
    setExecutorPref("");
  }, []);

  const handleCreate = useCallback(async () => {
    if (!name.trim() || !workspaceId) return;
    setSubmitting(true);
    try {
      const result = await createAgentInstance(workspaceId, {
        name: name.trim(),
        role,
        reportsTo: reportsTo || undefined,
        budgetMonthlyCents: budgetCents,
        maxConcurrentSessions: maxConcurrent,
        executorPreference: executorPref ? { type: executorPref } : undefined,
      } as Partial<AgentInstance>);
      if (result) {
        addAgentInstance(result as unknown as AgentInstance);
      }
      resetForm();
      onOpenChange(false);
    } finally {
      setSubmitting(false);
    }
  }, [
    name, role, reportsTo, budgetCents, maxConcurrent,
    executorPref, workspaceId, addAgentInstance, onOpenChange, resetForm,
  ]);

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="max-w-lg">
        <DialogHeader>
          <DialogTitle>Create Agent</DialogTitle>
        </DialogHeader>

        <div className="space-y-4">
          <div>
            <Label>Name</Label>
            <Input
              placeholder="e.g. Frontend Worker"
              value={name}
              onChange={(e) => setName(e.target.value)}
              className="mt-1"
              autoFocus
            />
          </div>

          <div className="flex gap-4">
            <div className="flex-1">
              <Label>Role</Label>
              <Select value={role} onValueChange={(v) => setRole(v as AgentRole)}>
                <SelectTrigger className="mt-1 cursor-pointer">
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  <SelectItem value="ceo" className="cursor-pointer">CEO</SelectItem>
                  <SelectItem value="worker" className="cursor-pointer">Worker</SelectItem>
                  <SelectItem value="specialist" className="cursor-pointer">Specialist</SelectItem>
                  <SelectItem value="assistant" className="cursor-pointer">Assistant</SelectItem>
                </SelectContent>
              </Select>
            </div>
            <div className="flex-1">
              <Label>Reports to</Label>
              <Select value={reportsTo || "__none__"} onValueChange={(v) => setReportsTo(v === "__none__" ? "" : v)}>
                <SelectTrigger className="mt-1 cursor-pointer">
                  <SelectValue placeholder="None (top-level)" />
                </SelectTrigger>
                <SelectContent>
                  <SelectItem value="__none__" className="cursor-pointer">None</SelectItem>
                  {agents.map((a) => (
                    <SelectItem key={a.id} value={a.id} className="cursor-pointer">
                      {a.name}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
            </div>
          </div>

          <div className="flex gap-4">
            <div className="flex-1">
              <Label>Monthly budget ($)</Label>
              <Input
                type="number"
                min={0}
                value={budgetCents / 100}
                onChange={(e) => setBudgetCents(Math.round(Number(e.target.value) * 100))}
                className="mt-1"
              />
            </div>
            <div className="flex-1">
              <Label>Max concurrent</Label>
              <Input
                type="number"
                min={1}
                max={10}
                value={maxConcurrent}
                onChange={(e) => setMaxConcurrent(Number(e.target.value))}
                className="mt-1"
              />
            </div>
          </div>

          <div>
            <Label>Executor preference</Label>
            <Select value={executorPref || "__inherit__"} onValueChange={(v) => setExecutorPref(v === "__inherit__" ? "" : v)}>
              <SelectTrigger className="mt-1 cursor-pointer">
                <SelectValue placeholder="Inherit from project/workspace" />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value="__inherit__" className="cursor-pointer">Inherit</SelectItem>
                <SelectItem value="local_pc" className="cursor-pointer">Local (standalone)</SelectItem>
                <SelectItem value="local_docker" className="cursor-pointer">Local Docker</SelectItem>
                <SelectItem value="sprites" className="cursor-pointer">Sprites (remote sandbox)</SelectItem>
              </SelectContent>
            </Select>
          </div>
        </div>

        <DialogFooter>
          <Button
            variant="ghost"
            onClick={() => onOpenChange(false)}
            className="cursor-pointer"
          >
            Cancel
          </Button>
          <Button
            onClick={handleCreate}
            disabled={!name.trim() || submitting}
            className="cursor-pointer"
          >
            {submitting ? "Creating..." : "Create Agent"}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
