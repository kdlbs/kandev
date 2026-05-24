"use client";

import { useState, useEffect, useMemo } from "react";
import { Label } from "@kandev/ui/label";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@kandev/ui/select";
import { useAppStore } from "@/components/state-provider";
import { useSettingsData } from "@/hooks/domains/settings/use-settings-data";
import { useWorkflows } from "@/hooks/use-workflows";
import { useRepositories } from "@/hooks/domains/workspace/use-repositories";
import { discoverRepositoriesAction } from "@/app/actions/workspaces";
import { listWorkflowSteps } from "@/lib/api/domains/workflow-api";
import type { LocalRepository, Repository } from "@/lib/types/http";
import type { ExecutionMode, TriggerType } from "@/lib/types/automation";

// RepositorySelection mirrors the task-create dialog's two-tier model: a
// registered workspace repository (keyed by id) OR a filesystem-discovered
// repo not yet registered (keyed by local path). The form holds whichever
// the user picked; save-time logic registers the discovered repo first to
// land an id on the automation row. "none" leaves the repository unset on
// the automation row — the orchestrator runs the task without a repo,
// which is the right choice for notification-style or side-effect-only
// automations.
export type RepositorySelection =
  | { kind: "none" }
  | { kind: "registered"; id: string }
  | { kind: "discovered"; path: string; name: string; defaultBranch: string };

type ConfigSectionProps = {
  workspaceId: string;
  workflowId: string;
  workflowStepId: string;
  agentProfileId: string;
  executorProfileId: string;
  repositorySelection: RepositorySelection;
  executionMode: ExecutionMode;
  conditionType: TriggerType | null;
  onWorkflowChange: (id: string) => void;
  onStepChange: (id: string) => void;
  onAgentProfileChange: (id: string) => void;
  onExecutorProfileChange: (id: string) => void;
  onRepositoryChange: (selection: RepositorySelection) => void;
  onExecutionModeChange: (mode: ExecutionMode) => void;
};

const REPO_NONE_OPTION_ID = "__none__";
const DISCOVERED_PREFIX = "path:";

function selectionToOptionId(sel: RepositorySelection): string {
  if (sel.kind === "registered") return sel.id;
  if (sel.kind === "discovered") return DISCOVERED_PREFIX + sel.path;
  return REPO_NONE_OPTION_ID;
}

function buildRepositoryItems(
  workspaceRepos: Repository[],
  discoveredRepos: LocalRepository[],
): Array<{ id: string; label: string }> {
  const registeredPaths = new Set(
    workspaceRepos
      .map((r) => r.local_path)
      .filter(Boolean)
      .map((p) => p.replace(/\/+$/, "")),
  );
  const items: Array<{ id: string; label: string }> = [
    { id: REPO_NONE_OPTION_ID, label: "None — no repository" },
  ];
  for (const r of workspaceRepos) {
    items.push({ id: r.id, label: r.name || `${r.provider_owner}/${r.provider_name}` });
  }
  for (const r of discoveredRepos) {
    if (registeredPaths.has(r.path.replace(/\/+$/, ""))) continue;
    items.push({ id: DISCOVERED_PREFIX + r.path, label: `${r.name} — ${r.path}` });
  }
  return items;
}

function pickSelectionFromOptionId(
  optionId: string,
  workspaceRepos: Repository[],
  discoveredRepos: LocalRepository[],
): RepositorySelection {
  if (optionId === REPO_NONE_OPTION_ID) return { kind: "none" };
  if (optionId.startsWith(DISCOVERED_PREFIX)) {
    const path = optionId.slice(DISCOVERED_PREFIX.length);
    const match = discoveredRepos.find((r) => r.path === path);
    return {
      kind: "discovered",
      path,
      name: match?.name ?? path.split("/").pop() ?? "New Repository",
      defaultBranch: match?.default_branch ?? "",
    };
  }
  const reg = workspaceRepos.find((r) => r.id === optionId);
  return reg ? { kind: "registered", id: reg.id } : { kind: "none" };
}

const EXECUTION_MODE_ITEMS = [
  { id: "task", label: "Task — creates a tracked kanban task" },
  { id: "run", label: "Run — fire-and-forget, hidden from kanban" },
];

function useDiscoveredRepositories(workspaceId: string) {
  const [items, setItems] = useState<LocalRepository[]>([]);
  useEffect(() => {
    if (!workspaceId) return;
    let cancelled = false;
    discoverRepositoriesAction(workspaceId)
      .then((res) => {
        if (cancelled) return;
        setItems(res.repositories ?? []);
      })
      .catch(() => {
        if (!cancelled) setItems([]);
      });
    return () => {
      cancelled = true;
    };
  }, [workspaceId]);
  return items;
}

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
  repositorySelection,
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
  const discoveredRepos = useDiscoveredRepositories(workspaceId);

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
  const isRunMode = executionMode === "run";
  const repositoryItems = useMemo(
    () => buildRepositoryItems(repositories, discoveredRepos),
    [repositories, discoveredRepos],
  );

  return (
    <div className="space-y-3">
      <Label className="text-xs uppercase tracking-wider text-muted-foreground">
        Configuration
      </Label>
      <div className="grid grid-cols-2 gap-4">
        <SelectField
          testId="execution-mode-selector"
          label="Execution Mode"
          value={executionMode}
          onChange={(v) => onExecutionModeChange(v as ExecutionMode)}
          placeholder="Select mode"
          items={EXECUTION_MODE_ITEMS}
        />
        {!isRunMode && (
          <WorkflowFields
            workflowId={workflowId}
            workflowStepId={workflowStepId}
            workflows={workflows}
            steps={steps}
            onWorkflowChange={onWorkflowChange}
            onStepChange={onStepChange}
          />
        )}
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
          value={selectionToOptionId(repositorySelection)}
          onChange={(v) =>
            onRepositoryChange(pickSelectionFromOptionId(v, repositories, discoveredRepos))
          }
          placeholder="Auto"
          items={repositoryItems}
          disabled={isPRTrigger}
          helpText={isPRTrigger ? "PR triggers always use the PR's own repository." : undefined}
        />
      </div>
    </div>
  );
}

function WorkflowFields({
  workflowId,
  workflowStepId,
  workflows,
  steps,
  onWorkflowChange,
  onStepChange,
}: {
  workflowId: string;
  workflowStepId: string;
  workflows: Array<{ id: string; name: string }>;
  steps: StepOption[];
  onWorkflowChange: (id: string) => void;
  onStepChange: (id: string) => void;
}) {
  // The step list is empty until a workflow is picked. Showing an empty
  // dropdown next to the workflow select invites users to click it first
  // and bounce off — keep the field in the DOM (so its testid is stable
  // for tooling) but disable it and surface a hint until a workflow is
  // chosen.
  const hasWorkflow = !!workflowId;
  return (
    <>
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
        placeholder={hasWorkflow ? "Select step" : "Pick a workflow first"}
        items={steps.map((s) => ({ id: s.id, label: s.name }))}
        disabled={!hasWorkflow}
        helpText={hasWorkflow ? undefined : "Pick a workflow above to load its steps."}
      />
    </>
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
