"use client";

import { useState, useCallback, useMemo } from "react";
import { Card, CardContent, CardHeader, CardTitle } from "@kandev/ui/card";
import { Input } from "@kandev/ui/input";
import { Label } from "@kandev/ui/label";
import { Button } from "@kandev/ui/button";
import { Badge } from "@kandev/ui/badge";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@kandev/ui/select";
import { toast } from "sonner";
import { useAppStore } from "@/components/state-provider";
import { updateAgentProfile } from "@/lib/api/domains/office-api";
import type { AgentProfile, AgentRole } from "@/lib/state/slices/office/types";
import { AgentConfigCliCard } from "./agent-config-cli-card";
import { AgentRoutingCard } from "./agent-routing-card";

type AgentConfigurationTabProps = {
  agent: AgentProfile;
};

const FALLBACK_ROLES: Array<{ id: string; label: string }> = [
  { id: "ceo", label: "CEO" },
  { id: "worker", label: "Worker" },
  { id: "specialist", label: "Specialist" },
  { id: "assistant", label: "Assistant" },
  { id: "security", label: "Security" },
  { id: "qa", label: "QA" },
  { id: "devops", label: "DevOps" },
];

const FALLBACK_EXECUTOR_TYPES: Array<{ id: string; label: string }> = [
  { id: "local_pc", label: "Local (standalone)" },
  { id: "local_docker", label: "Local Docker" },
  { id: "sprites", label: "Sprites (remote sandbox)" },
];

const CAPABILITY_LABELS: Record<string, string> = {
  post_comment: "Post comment",
  update_task_status: "Update task status",
  create_subtask: "Create subtask",
  create_agent: "Create agent",
  request_approval: "Request approval",
  read_memory: "Read memory",
  write_memory: "Write memory",
  list_skills: "List skills",
  spawn_agent_run: "Spawn run",
  modify_agents: "Modify agents",
  delete_skills: "Delete skills",
};

type FormState = {
  name: string;
  role: AgentRole;
  agentProfileId: string;
  budgetMonthlyCents: number;
  maxConcurrentSessions: number;
  executorType: string;
};

function initialForm(agent: AgentProfile): FormState {
  return {
    name: agent.name,
    role: agent.role,
    agentProfileId: agent.agentProfileId || agent.id,
    budgetMonthlyCents: agent.budgetMonthlyCents,
    maxConcurrentSessions: agent.maxConcurrentSessions,
    executorType: agent.executorPreference?.type ?? "",
  };
}

export function AgentConfigurationTab({ agent }: AgentConfigurationTabProps) {
  const meta = useAppStore((s) => s.office.meta);
  const updateStore = useAppStore((s) => s.updateOfficeAgentProfile);
  const allOfficeAgents = useAppStore((s) => s.office.agentProfiles);

  const roles = meta?.roles.map((r) => ({ id: r.id, label: r.label })) ?? FALLBACK_ROLES;
  const executorTypes =
    meta?.executorTypes.map((e) => ({ id: e.id, label: e.label })) ?? FALLBACK_EXECUTOR_TYPES;

  const [form, setForm] = useState<FormState>(() => initialForm(agent));
  const [saving, setSaving] = useState(false);
  const [dirty, setDirty] = useState(false);

  const patch = useCallback((p: Partial<FormState>) => {
    setForm((prev) => ({ ...prev, ...p }));
    setDirty(true);
  }, []);

  const reportsToAgent = useMemo(
    () => allOfficeAgents.find((a) => a.id === agent.reportsTo),
    [allOfficeAgents, agent.reportsTo],
  );

  const handleSave = useCallback(async () => {
    setSaving(true);
    try {
      const update: Partial<AgentProfile> = {
        name: form.name,
        role: form.role,
        agentProfileId: form.agentProfileId || undefined,
        budgetMonthlyCents: form.budgetMonthlyCents,
        maxConcurrentSessions: form.maxConcurrentSessions,
        executorPreference: form.executorType ? { type: form.executorType } : undefined,
      };
      await updateAgentProfile(agent.id, update);
      updateStore(agent.id, update);
      setDirty(false);
      toast.success("Agent configuration updated");
    } catch (err) {
      toast.error(err instanceof Error ? err.message : "Failed to update agent");
    } finally {
      setSaving(false);
    }
  }, [agent.id, form, updateStore]);

  return (
    <div className="space-y-4 mt-4" data-testid="agent-configuration-tab">
      <IdentityCard
        name={form.name}
        role={form.role}
        roles={roles}
        reportsToName={reportsToAgent?.name ?? "None"}
        onNameChange={(v) => patch({ name: v })}
        onRoleChange={(v) => patch({ role: v })}
      />
      <CapabilityPreviewCard agent={agent} role={form.role} />
      <AgentConfigCliCard
        agentProfileId={form.agentProfileId}
        currentAgent={agent}
        onAgentProfileChange={(v) => patch({ agentProfileId: v })}
      />
      <OrchestrationCard
        budgetCents={form.budgetMonthlyCents}
        maxConcurrent={form.maxConcurrentSessions}
        executorType={form.executorType}
        executorTypes={executorTypes}
        onBudgetChange={(v) => patch({ budgetMonthlyCents: v })}
        onMaxConcurrentChange={(v) => patch({ maxConcurrentSessions: v })}
        onExecutorChange={(v) => patch({ executorType: v })}
      />
      <AgentRoutingCard agentId={agent.id} />
      {dirty && (
        <div className="flex justify-end">
          <Button onClick={handleSave} disabled={saving} className="cursor-pointer">
            {saving ? "Saving..." : "Save Configuration"}
          </Button>
        </div>
      )}
    </div>
  );
}

function CapabilityPreviewCard({ agent, role }: { agent: AgentProfile; role: AgentRole }) {
  const capabilities = effectiveCapabilities(agent, role);
  return (
    <Card>
      <CardHeader>
        <CardTitle className="text-sm">Runtime capabilities</CardTitle>
        <p className="text-xs text-muted-foreground">
          Effective actions this agent can request during Office runs.
        </p>
      </CardHeader>
      <CardContent>
        <div className="flex flex-wrap gap-2" data-testid="agent-capability-preview">
          {capabilities.map((key) => (
            <Badge key={key} variant="secondary">
              {CAPABILITY_LABELS[key] ?? key}
            </Badge>
          ))}
        </div>
      </CardContent>
    </Card>
  );
}

function effectiveCapabilities(agent: AgentProfile, role: AgentRole): string[] {
  const permissions = agent.permissions ?? {};
  const allowed = new Set<string>([
    "post_comment",
    "update_task_status",
    "create_subtask",
    "read_memory",
    "write_memory",
    "list_skills",
  ]);
  if (role === "ceo" || permissions.can_create_agents === true) allowed.add("create_agent");
  if (permissions.can_approve === true) allowed.add("request_approval");
  if (permissions.can_spawn_agent_run === true) allowed.add("spawn_agent_run");
  if (permissions.can_modify_agents === true) allowed.add("modify_agents");
  if (permissions.can_delete_skills === true) allowed.add("delete_skills");
  return Object.keys(CAPABILITY_LABELS).filter((key) => allowed.has(key));
}

function IdentityCard({
  name,
  role,
  roles,
  reportsToName,
  onNameChange,
  onRoleChange,
}: {
  name: string;
  role: AgentRole;
  roles: Array<{ id: string; label: string }>;
  reportsToName: string;
  onNameChange: (v: string) => void;
  onRoleChange: (v: AgentRole) => void;
}) {
  return (
    <Card>
      <CardHeader>
        <CardTitle className="text-sm">Identity</CardTitle>
        <p className="text-xs text-muted-foreground">
          Name, role, and reporting structure for this agent.
        </p>
      </CardHeader>
      <CardContent className="space-y-3">
        <div>
          <Label htmlFor="cfg-name">Name</Label>
          <Input
            id="cfg-name"
            value={name}
            onChange={(e) => onNameChange(e.target.value)}
            className="mt-1"
          />
        </div>
        <div className="flex gap-4">
          <div className="flex-1">
            <Label>Role</Label>
            <Select value={role} onValueChange={(v) => onRoleChange(v as AgentRole)}>
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
          </div>
          <div className="flex-1">
            <Label>Reports to</Label>
            <Input value={reportsToName} disabled className="mt-1" />
            <p className="text-xs text-muted-foreground mt-1">
              Edit org chart from the agents list.
            </p>
          </div>
        </div>
      </CardContent>
    </Card>
  );
}

function OrchestrationCard({
  budgetCents,
  maxConcurrent,
  executorType,
  executorTypes,
  onBudgetChange,
  onMaxConcurrentChange,
  onExecutorChange,
}: {
  budgetCents: number;
  maxConcurrent: number;
  executorType: string;
  executorTypes: Array<{ id: string; label: string }>;
  onBudgetChange: (v: number) => void;
  onMaxConcurrentChange: (v: number) => void;
  onExecutorChange: (v: string) => void;
}) {
  return (
    <Card>
      <CardHeader>
        <CardTitle className="text-sm">Orchestration</CardTitle>
        <p className="text-xs text-muted-foreground">
          Budget cap, concurrency, and execution environment.
        </p>
      </CardHeader>
      <CardContent className="space-y-3">
        <div className="flex gap-4">
          <div className="flex-1">
            <Label>Monthly budget ($)</Label>
            <Input
              type="number"
              min={0}
              value={budgetCents / 100}
              onChange={(e) => onBudgetChange(Math.round(Number(e.target.value) * 100))}
              className="mt-1"
            />
          </div>
          <div className="flex-1">
            <Label>Max concurrent sessions</Label>
            <Input
              type="number"
              min={1}
              max={10}
              value={maxConcurrent}
              onChange={(e) => onMaxConcurrentChange(Number(e.target.value))}
              className="mt-1"
            />
          </div>
        </div>
        <div>
          <Label>Executor preference</Label>
          <Select
            value={executorType || "__inherit__"}
            onValueChange={(v) => onExecutorChange(v === "__inherit__" ? "" : v)}
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
        </div>
      </CardContent>
    </Card>
  );
}
