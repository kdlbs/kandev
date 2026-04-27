"use client";

import { useState, useCallback } from "react";
import { Button } from "@kandev/ui/button";
import { Input } from "@kandev/ui/input";
import { Label } from "@kandev/ui/label";
import { Dialog, DialogContent, DialogHeader, DialogTitle, DialogFooter } from "@kandev/ui/dialog";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@kandev/ui/select";
import { toast } from "sonner";
import { useAppStore } from "@/components/state-provider";
import { createAgentInstance } from "@/lib/api/domains/orchestrate-api";
import type { AgentRole, AgentInstance } from "@/lib/state/slices/orchestrate/types";

type CreateAgentDialogProps = {
  open: boolean;
  onOpenChange: (open: boolean) => void;
};

type FormState = {
  name: string;
  role: AgentRole;
  reportsTo: string;
  budgetCents: number;
  maxConcurrent: number;
  executorPref: string;
};

const INITIAL_STATE: FormState = {
  name: "",
  role: "worker",
  reportsTo: "",
  budgetCents: 0,
  maxConcurrent: 1,
  executorPref: "",
};

function NameField({ value, onChange }: { value: string; onChange: (v: string) => void }) {
  return (
    <div>
      <Label>Name</Label>
      <Input
        placeholder="e.g. Frontend Worker"
        value={value}
        onChange={(e) => onChange(e.target.value)}
        className="mt-1"
        autoFocus
      />
      <p className="text-xs text-muted-foreground mt-1">
        A unique name for this agent (e.g. CEO, Frontend Worker)
      </p>
    </div>
  );
}

function RoleAndReports({
  role,
  reportsTo,
  agents,
  roles,
  onChange,
}: {
  role: AgentRole;
  reportsTo: string;
  agents: AgentInstance[];
  roles: Array<{ id: string; label: string }>;
  onChange: (patch: Partial<FormState>) => void;
}) {
  return (
    <div className="flex gap-4">
      <div className="flex-1">
        <Label>Role</Label>
        <Select value={role} onValueChange={(v) => onChange({ role: v as AgentRole })}>
          <SelectTrigger className="mt-1 cursor-pointer">
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            {roles.map((r) => (
              <SelectItem key={r.id} value={r.id} className="cursor-pointer">
                {r.label}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>
        <p className="text-xs text-muted-foreground mt-1">
          CEO manages other agents, workers execute tasks
        </p>
      </div>
      <div className="flex-1">
        <Label>Reports to</Label>
        <Select
          value={reportsTo || "__none__"}
          onValueChange={(v) => onChange({ reportsTo: v === "__none__" ? "" : v })}
        >
          <SelectTrigger className="mt-1 cursor-pointer">
            <SelectValue placeholder="None (top-level)" />
          </SelectTrigger>
          <SelectContent>
            <SelectItem value="__none__" className="cursor-pointer">
              None
            </SelectItem>
            {agents.map((a) => (
              <SelectItem key={a.id} value={a.id} className="cursor-pointer">
                {a.name}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>
        <p className="text-xs text-muted-foreground mt-1">Which agent manages this one</p>
      </div>
    </div>
  );
}

function BudgetAndConcurrency({
  budgetCents,
  maxConcurrent,
  onChange,
}: {
  budgetCents: number;
  maxConcurrent: number;
  onChange: (patch: Partial<FormState>) => void;
}) {
  return (
    <div className="flex gap-4">
      <div className="flex-1">
        <Label>Monthly budget ($)</Label>
        <Input
          type="number"
          min={0}
          value={budgetCents / 100}
          onChange={(e) => onChange({ budgetCents: Math.round(Number(e.target.value) * 100) })}
          className="mt-1"
        />
        <p className="text-xs text-muted-foreground mt-1">
          Monthly spending limit ($0 = unlimited)
        </p>
      </div>
      <div className="flex-1">
        <Label>Max concurrent</Label>
        <Input
          type="number"
          min={1}
          max={10}
          value={maxConcurrent}
          onChange={(e) => onChange({ maxConcurrent: Number(e.target.value) })}
          className="mt-1"
        />
        <p className="text-xs text-muted-foreground mt-1">
          How many tasks this agent can run at once
        </p>
      </div>
    </div>
  );
}

function ExecutorPreferenceField({
  value,
  executorTypes,
  onChange,
}: {
  value: string;
  executorTypes: Array<{ id: string; label: string }>;
  onChange: (v: string) => void;
}) {
  return (
    <div>
      <Label>Executor preference</Label>
      <Select
        value={value || "__inherit__"}
        onValueChange={(v) => onChange(v === "__inherit__" ? "" : v)}
      >
        <SelectTrigger className="mt-1 cursor-pointer">
          <SelectValue placeholder="Inherit from project/workspace" />
        </SelectTrigger>
        <SelectContent>
          <SelectItem value="__inherit__" className="cursor-pointer">
            Inherit
          </SelectItem>
          {executorTypes.map((et) => (
            <SelectItem key={et.id} value={et.id} className="cursor-pointer">
              {et.label}
            </SelectItem>
          ))}
        </SelectContent>
      </Select>
      <p className="text-xs text-muted-foreground mt-1">
        How agent sessions run (inherit uses project/workspace default)
      </p>
    </div>
  );
}

const FALLBACK_ROLES = [
  { id: "ceo", label: "CEO" },
  { id: "worker", label: "Worker" },
  { id: "specialist", label: "Specialist" },
  { id: "assistant", label: "Assistant" },
];

const FALLBACK_EXECUTOR_TYPES = [
  { id: "local_pc", label: "Local (standalone)" },
  { id: "local_docker", label: "Local Docker" },
  { id: "sprites", label: "Sprites (remote sandbox)" },
];

export function CreateAgentDialog({ open, onOpenChange }: CreateAgentDialogProps) {
  const workspaceId = useAppStore((s) => s.workspaces.activeId);
  const agents = useAppStore((s) => s.orchestrate.agentInstances);
  const meta = useAppStore((s) => s.orchestrate.meta);
  const addAgentInstance = useAppStore((s) => s.addAgentInstance);

  const roles = meta?.roles.map((r) => ({ id: r.id, label: r.label })) ?? FALLBACK_ROLES;
  const executorTypes =
    meta?.executorTypes.map((e) => ({ id: e.id, label: e.label })) ?? FALLBACK_EXECUTOR_TYPES;

  const [state, setState] = useState<FormState>(INITIAL_STATE);
  const [submitting, setSubmitting] = useState(false);

  const handleChange = useCallback(
    (patch: Partial<FormState>) => setState((prev) => ({ ...prev, ...patch })),
    [],
  );

  const handleCreate = useCallback(async () => {
    if (!state.name.trim() || !workspaceId) return;
    setSubmitting(true);
    try {
      const result = await createAgentInstance(workspaceId, {
        name: state.name.trim(),
        role: state.role,
        reportsTo: state.reportsTo || undefined,
        budgetMonthlyCents: state.budgetCents,
        maxConcurrentSessions: state.maxConcurrent,
        executorPreference: state.executorPref ? { type: state.executorPref } : undefined,
      } as Partial<AgentInstance>);
      if (result) {
        addAgentInstance(result as unknown as AgentInstance);
      }
      setState(INITIAL_STATE);
      onOpenChange(false);
      toast.success("Agent created");
    } catch (err) {
      toast.error(err instanceof Error ? err.message : "Failed to create agent");
    } finally {
      setSubmitting(false);
    }
  }, [state, workspaceId, addAgentInstance, onOpenChange]);

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="max-w-lg">
        <DialogHeader>
          <DialogTitle>Create Agent</DialogTitle>
        </DialogHeader>
        <div className="space-y-4">
          <NameField value={state.name} onChange={(v) => handleChange({ name: v })} />
          <RoleAndReports
            role={state.role}
            reportsTo={state.reportsTo}
            agents={agents}
            roles={roles}
            onChange={handleChange}
          />
          <BudgetAndConcurrency
            budgetCents={state.budgetCents}
            maxConcurrent={state.maxConcurrent}
            onChange={handleChange}
          />
          <ExecutorPreferenceField
            value={state.executorPref}
            executorTypes={executorTypes}
            onChange={(v) => handleChange({ executorPref: v })}
          />
        </div>
        <DialogFooter>
          <Button variant="ghost" onClick={() => onOpenChange(false)} className="cursor-pointer">
            Cancel
          </Button>
          <Button
            onClick={handleCreate}
            disabled={!state.name.trim() || submitting}
            className="cursor-pointer"
          >
            {submitting ? "Creating..." : "Create Agent"}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
