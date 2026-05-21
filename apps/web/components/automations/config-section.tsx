"use client";

import { useState, useEffect, useMemo } from "react";
import { Label } from "@kandev/ui/label";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@kandev/ui/select";
import { useAppStore } from "@/components/state-provider";
import { useSettingsData } from "@/hooks/domains/settings/use-settings-data";
import { useWorkflows } from "@/hooks/use-workflows";
import { useRepositories } from "@/hooks/domains/workspace/use-repositories";
import { listWorkflowSteps } from "@/lib/api/domains/workflow-api";
import type { ExecutionMode, TriggerType } from "@/lib/types/automation";

type ConfigSectionProps = {
  workspaceId: string;
  workflowId: string;
  workflowStepId: string;
  agentProfileId: string;
  executorProfileId: string;
  repositoryId: string;
  executionMode: ExecutionMode;
  conditionType: TriggerType | null;
  onWorkflowChange: (id: string) => void;
  onStepChange: (id: string) => void;
  onAgentProfileChange: (id: string) => void;
  onExecutorProfileChange: (id: string) => void;
  onRepositoryChange: (id: string) => void;
  onExecutionModeChange: (mode: ExecutionMode) => void;
};

const REPO_AUTO_OPTION_ID = "__auto__";

type RepoLike = { id: string; name: string; provider_owner: string; provider_name: string };

function buildRepositoryItems(repositories: RepoLike[]): Array<{ id: string; label: string }> {
  const items: Array<{ id: string; label: string }> = [
    { id: REPO_AUTO_OPTION_ID, label: "Auto — first workspace repo" },
  ];
  for (const r of repositories) {
    items.push({ id: r.id, label: r.name || `${r.provider_owner}/${r.provider_name}` });
  }
  return items;
}

const EXECUTION_MODE_ITEMS = [
  { id: "task", label: "Task — creates a tracked kanban task" },
  { id: "run", label: "Run — fire-and-forget, hidden from kanban" },
];

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
  repositoryId,
  executionMode,
  conditionType,
  onWorkflowChange,
  onStepChange,
  onAgentProfileChange,
  onExecutorProfileChange,
  onRepositoryChange,
  onExecutionModeChange,
}: ConfigSectionProps) {
  useSettingsData(true);
  useWorkflows(workspaceId, true);
  const { repositories } = useRepositories(workspaceId, true);

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
  const isPRTrigger = conditionType === "github_pr";
  const repositoryItems = useMemo(() => buildRepositoryItems(repositories), [repositories]);

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
        <SelectField
          testId="repository-selector"
          label="Repository"
          value={repositoryId || REPO_AUTO_OPTION_ID}
          onChange={(v) => onRepositoryChange(v === REPO_AUTO_OPTION_ID ? "" : v)}
          placeholder="Auto"
          items={repositoryItems}
          disabled={isPRTrigger}
          helpText={isPRTrigger ? "PR triggers always use the PR's own repository." : undefined}
        />
        <SelectField
          testId="execution-mode-selector"
          label="Execution Mode"
          value={executionMode}
          onChange={(v) => onExecutionModeChange(v as ExecutionMode)}
          placeholder="Select mode"
          items={EXECUTION_MODE_ITEMS}
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
  disabled,
  helpText,
}: {
  testId?: string;
  label: string;
  value: string;
  onChange: (value: string) => void;
  placeholder: string;
  items: Array<{ id: string; label: string }>;
  disabled?: boolean;
  helpText?: string;
}) {
  return (
    <div className="space-y-1.5">
      <Label className="text-xs">{label}</Label>
      <Select value={value || undefined} onValueChange={onChange} disabled={disabled}>
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
      {helpText && <p className="text-[10px] text-muted-foreground">{helpText}</p>}
    </div>
  );
}
