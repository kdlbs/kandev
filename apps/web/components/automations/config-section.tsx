"use client";

import { useState, useEffect, useMemo } from "react";
import { Label } from "@kandev/ui/label";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@kandev/ui/select";
import { useAppStore } from "@/components/state-provider";
import { useSettingsData } from "@/hooks/domains/settings/use-settings-data";
import { useWorkflows } from "@/hooks/use-workflows";
import { listWorkflowSteps } from "@/lib/api/domains/workflow-api";

type ConfigSectionProps = {
  workspaceId: string;
  workflowId: string;
  workflowStepId: string;
  agentProfileId: string;
  executorProfileId: string;
  onWorkflowChange: (id: string) => void;
  onStepChange: (id: string) => void;
  onAgentProfileChange: (id: string) => void;
  onExecutorProfileChange: (id: string) => void;
};

type StepOption = { id: string; name: string };

function useWorkflowSteps(workflowId: string) {
  const [steps, setSteps] = useState<StepOption[]>([]);

  useEffect(() => {
    if (!workflowId) return;
    let cancelled = false;
    listWorkflowSteps(workflowId)
      .then((response) => {
        if (cancelled) return;
        const sorted = [...response.steps].sort((a, b) => a.position - b.position);
        setSteps(sorted.map((s) => ({ id: s.id, name: s.name })));
      })
      .catch(() => {
        if (!cancelled) setSteps([]);
      });
    return () => {
      cancelled = true;
    };
  }, [workflowId]);

  return steps;
}

export function ConfigSection({
  workspaceId,
  workflowId,
  workflowStepId,
  agentProfileId,
  executorProfileId,
  onWorkflowChange,
  onStepChange,
  onAgentProfileChange,
  onExecutorProfileChange,
}: ConfigSectionProps) {
  useSettingsData(true);
  useWorkflows(workspaceId, true);

  const workflows = useAppStore((state) => state.workflows.items);
  const agentProfiles = useAppStore((state) => state.agentProfiles.items);
  const executors = useAppStore((state) => state.executors.items);
  const steps = useWorkflowSteps(workflowId);

  const filteredAgentProfiles = useMemo(
    () => agentProfiles.filter((p) => !p.cli_passthrough),
    [agentProfiles],
  );
  const allExecutorProfiles = useMemo(
    () => executors.filter((e) => e.type !== "local").flatMap((e) => e.profiles ?? []),
    [executors],
  );

  return (
    <div className="space-y-3">
      <Label className="text-xs uppercase tracking-wider text-muted-foreground">
        Configuration
      </Label>
      <div className="grid grid-cols-2 gap-4">
        <SelectField
          testId="workflow-selector"
          label="Workflow"
          value={workflowId}
          onChange={onWorkflowChange}
          placeholder="Select workflow"
          items={workflows.map((w) => ({ id: w.id, label: w.name }))}
        />
        <SelectField
          testId="workflow-step-selector"
          label="Workflow Step"
          value={workflowStepId}
          onChange={onStepChange}
          placeholder="Select step"
          items={steps.map((s) => ({ id: s.id, label: s.name }))}
        />
        <SelectField
          label="Agent Profile"
          value={agentProfileId}
          onChange={onAgentProfileChange}
          placeholder="Select agent"
          items={filteredAgentProfiles.map((p) => ({
            id: p.id,
            label: p.label,
          }))}
        />
        <SelectField
          label="Executor Profile"
          value={executorProfileId}
          onChange={onExecutorProfileChange}
          placeholder="Select executor"
          items={allExecutorProfiles.map((p) => ({ id: p.id, label: p.name }))}
        />
      </div>
    </div>
  );
}

function SelectField({
  testId,
  label,
  value,
  onChange,
  placeholder,
  items,
}: {
  testId?: string;
  label: string;
  value: string;
  onChange: (value: string) => void;
  placeholder: string;
  items: Array<{ id: string; label: string }>;
}) {
  return (
    <div className="space-y-1.5">
      <Label className="text-xs">{label}</Label>
      <Select value={value || undefined} onValueChange={onChange}>
        <SelectTrigger data-testid={testId} className="cursor-pointer">
          <SelectValue placeholder={placeholder} />
        </SelectTrigger>
        <SelectContent>
          {items.map((item) => (
            <SelectItem key={item.id} value={item.id}>
              {item.label}
            </SelectItem>
          ))}
        </SelectContent>
      </Select>
    </div>
  );
}
