"use client";

import { useState, useCallback } from "react";
import { Card, CardContent, CardHeader, CardTitle } from "@kandev/ui/card";
import { Input } from "@kandev/ui/input";
import { Label } from "@kandev/ui/label";
import { Button } from "@kandev/ui/button";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@kandev/ui/select";
import { toast } from "sonner";
import { useAppStore } from "@/components/state-provider";
import { updateAgentInstance } from "@/lib/api/domains/orchestrate-api";
import type { AgentInstance, AgentRole } from "@/lib/state/slices/orchestrate/types";

type AgentOverviewTabProps = {
  agent: AgentInstance;
};

export function AgentOverviewTab({ agent }: AgentOverviewTabProps) {
  const agents = useAppStore((s) => s.orchestrate.agentInstances);
  const updateStore = useAppStore((s) => s.updateAgentInstance);

  const [name, setName] = useState(agent.name);
  const [role, setRole] = useState<AgentRole>(agent.role);
  const [budget, setBudget] = useState(agent.budgetMonthlyCents / 100);
  const [maxConcurrent, setMaxConcurrent] = useState(agent.maxConcurrentSessions);
  const [executorType, setExecutorType] = useState(agent.executorPreference?.type ?? "");
  const [saving, setSaving] = useState(false);
  const [dirty, setDirty] = useState(false);

  const markDirty = useCallback(() => setDirty(true), []);

  const handleSave = useCallback(async () => {
    setSaving(true);
    try {
      await updateAgentInstance(agent.id, {
        name,
        role,
        budgetMonthlyCents: Math.round(budget * 100),
        maxConcurrentSessions: maxConcurrent,
        executorPreference: executorType ? { type: executorType } : undefined,
      } as Partial<AgentInstance>);
      updateStore(agent.id, {
        name,
        role,
        budgetMonthlyCents: Math.round(budget * 100),
        maxConcurrentSessions: maxConcurrent,
        executorPreference: executorType ? { type: executorType } : undefined,
      });
      setDirty(false);
      toast.success("Agent updated");
    } catch (err) {
      toast.error(err instanceof Error ? err.message : "Failed to update agent");
    } finally {
      setSaving(false);
    }
  }, [agent.id, name, role, budget, maxConcurrent, executorType, updateStore]);

  const reportsToAgent = agents.find((a) => a.id === agent.reportsTo);

  return (
    <div className="space-y-4 mt-4">
      <Card>
        <CardHeader>
          <CardTitle className="text-sm">Identity</CardTitle>
        </CardHeader>
        <CardContent className="space-y-3">
          <div>
            <Label>Name</Label>
            <Input
              value={name}
              onChange={(e) => { setName(e.target.value); markDirty(); }}
              className="mt-1"
            />
          </div>
          <div className="flex gap-4">
            <div className="flex-1">
              <Label>Role</Label>
              <Select value={role} onValueChange={(v) => { setRole(v as AgentRole); markDirty(); }}>
                <SelectTrigger className="mt-1 cursor-pointer"><SelectValue /></SelectTrigger>
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
              <Input
                value={reportsToAgent?.name ?? "None"}
                disabled
                className="mt-1"
              />
            </div>
          </div>
        </CardContent>
      </Card>

      <Card>
        <CardHeader>
          <CardTitle className="text-sm">Configuration</CardTitle>
        </CardHeader>
        <CardContent className="space-y-3">
          <div className="flex gap-4">
            <div className="flex-1">
              <Label>Monthly budget ($)</Label>
              <Input
                type="number"
                min={0}
                value={budget}
                onChange={(e) => { setBudget(Number(e.target.value)); markDirty(); }}
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
                onChange={(e) => { setMaxConcurrent(Number(e.target.value)); markDirty(); }}
                className="mt-1"
              />
            </div>
          </div>
          <div>
            <Label>Executor preference</Label>
            <Select
              value={executorType || "__inherit__"}
              onValueChange={(v) => { setExecutorType(v === "__inherit__" ? "" : v); markDirty(); }}
            >
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
        </CardContent>
      </Card>

      {dirty && (
        <div className="flex justify-end">
          <Button onClick={handleSave} disabled={saving} className="cursor-pointer">
            {saving ? "Saving..." : "Save Changes"}
          </Button>
        </div>
      )}
    </div>
  );
}
