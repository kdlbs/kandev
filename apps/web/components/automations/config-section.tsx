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
import type { ExecutorProfile, LocalRepository, Repository } from "@/lib/types/http";
import type { ExecutionMode, TriggerType } from "@/lib/types/automation";
import { getMultiRepoExecutorDisabledReason } from "@/components/task-create-dialog-multi-repo-guard";
import { RequiredFieldLabel } from "./required-field-label";
import { AutomationRepositoryRows } from "./automation-repository-rows";
import {
  buildRepositoryItems,
  pickSelectionFromOptionId,
  resolveExecutorType,
  selectionToOptionId,
  type RepositorySelection,
} from "./automation-repository-selection";

export type { RepositorySelection } from "./automation-repository-selection";
export { buildRepositoryItems, pickSelectionFromOptionId, selectionToOptionId };

// getExecutorItemDisabledReason gates the Executor Profile picker: once two
// or more repositories are selected, executor types that can't launch a
// multi-repository task are disabled — mirrors the task-creation dialog's
// pickExecutorDisabledReason (task-create-dialog-computed.ts), reusing the
// same shared predicate so the two surfaces never drift.
export function getExecutorItemDisabledReason(
  executorType: string | null | undefined,
  repositorySelections: RepositorySelection[],
): string | null {
  if (repositorySelections.length <= 1) return null;
  return getMultiRepoExecutorDisabledReason(executorType);
}

type ConfigSectionProps = {
  workspaceId: string;
  workflowId: string;
  workflowStepId: string;
  agentProfileId: string;
  executorProfileId: string;
  repositorySelections: RepositorySelection[];
  executionMode: ExecutionMode;
  conditionType: TriggerType | null;
  dirtyFields?: {
    executionMode: boolean;
    workflowId: boolean;
    workflowStepId: boolean;
    agentProfileId: boolean;
    executorProfileId: boolean;
    repositorySelections: boolean;
  };
  onWorkflowChange: (id: string) => void;
  onStepChange: (id: string) => void;
  onAgentProfileChange: (id: string) => void;
  onExecutorProfileChange: (id: string) => void;
  onRepositoriesChange: (selections: RepositorySelection[]) => void;
  onExecutionModeChange: (mode: ExecutionMode) => void;
};

const CLEAN_FIELDS = {
  executionMode: false,
  workflowId: false,
  workflowStepId: false,
  agentProfileId: false,
  executorProfileId: false,
  repositorySelections: false,
};

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

function getWorkflowStepHelpText(workflowId: string, workflowStepId: string): string | undefined {
  if (!workflowId) return "Select a workflow before choosing a step.";
  if (!workflowStepId) return "Select a workflow step to enable saving.";
  return undefined;
}

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

type AgentProfileLike = { id: string; label: string; cli_passthrough?: boolean };
type ExecutorLike = { type: string; name: string; profiles?: ExecutorProfile[] };

// useConfigSectionComputed derives the Agent Profile / Executor Profile /
// Repository picker item lists and the executor's multi-repo capability.
// Pulled out of ConfigSection to keep that component under the
// function-length lint cap.
function useConfigSectionComputed({
  agentProfiles,
  executors,
  executorProfileId,
  repositorySelections,
  repositories,
  discoveredRepos,
}: {
  agentProfiles: AgentProfileLike[];
  executors: ExecutorLike[];
  executorProfileId: string;
  repositorySelections: RepositorySelection[];
  repositories: Repository[];
  discoveredRepos: LocalRepository[];
}) {
  const filteredAgentProfiles = agentProfiles.filter((profile) => !profile.cli_passthrough);
  // Profiles returned by the executors list/boot payload don't always carry
  // their own executor_type/executor_name (only the settings > Executors
  // page's local mapper attaches those today) — fall back to the parent
  // executor's fields, mirroring task-create-dialog-computed.ts's identical
  // enrichment so the two surfaces never disagree on an executor's type.
  const allExecutorProfiles = executors
    .filter((executor) => executor.type !== "local")
    .flatMap((executor) =>
      (executor.profiles ?? []).map((p) => ({
        ...p,
        executor_type: p.executor_type ?? executor.type,
        executor_name: p.executor_name ?? executor.name,
      })),
    );
  const supportsMultiRepo =
    getMultiRepoExecutorDisabledReason(resolveExecutorType(executors, executorProfileId)) === null;
  const executorItems = allExecutorProfiles.map((p) => {
    const disabledReason = getExecutorItemDisabledReason(p.executor_type, repositorySelections);
    return {
      id: p.id,
      label: p.name,
      disabled: disabledReason !== null,
      disabledReason: disabledReason ?? undefined,
    };
  });
  const singleRepositoryItems = useMemo(
    () => buildRepositoryItems(repositories, discoveredRepos),
    [repositories, discoveredRepos],
  );
  return { filteredAgentProfiles, executorItems, supportsMultiRepo, singleRepositoryItems };
}

export function ConfigSection({
  workspaceId,
  workflowId,
  workflowStepId,
  agentProfileId,
  executorProfileId,
  repositorySelections,
  executionMode,
  conditionType,
  dirtyFields = CLEAN_FIELDS,
  onWorkflowChange,
  onStepChange,
  onAgentProfileChange,
  onExecutorProfileChange,
  onRepositoriesChange,
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
  const isPRTrigger = conditionType === "github_pr";
  const isRunMode = executionMode === "run";

  const { filteredAgentProfiles, executorItems, supportsMultiRepo, singleRepositoryItems } =
    useConfigSectionComputed({
      agentProfiles,
      executors,
      executorProfileId,
      repositorySelections,
      repositories,
      discoveredRepos,
    });

  return (
    <div className="space-y-3">
      <Label className="text-xs uppercase tracking-wider text-muted-foreground">
        Configuration
      </Label>
      <div className="grid grid-cols-1 gap-4 sm:grid-cols-2">
        <SelectField
          testId="execution-mode-selector"
          label="Execution Mode"
          value={executionMode}
          isDirty={dirtyFields.executionMode}
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
            workflowDirty={dirtyFields.workflowId}
            workflowStepDirty={dirtyFields.workflowStepId}
            onWorkflowChange={onWorkflowChange}
            onStepChange={onStepChange}
          />
        )}
        <SelectField
          label="Agent Profile"
          value={agentProfileId}
          isDirty={dirtyFields.agentProfileId}
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
          isDirty={dirtyFields.executorProfileId}
          onChange={onExecutorProfileChange}
          placeholder="Select executor"
          items={executorItems}
        />
        <RepositoryPickerField
          supportsMultiRepo={supportsMultiRepo}
          isPRTrigger={isPRTrigger}
          repositories={repositories}
          discoveredRepos={discoveredRepos}
          repositorySelections={repositorySelections}
          singleRepositoryItems={singleRepositoryItems}
          isDirty={dirtyFields.repositorySelections}
          onRepositoriesChange={onRepositoriesChange}
        />
      </div>
    </div>
  );
}

// RepositoryPickerField branches between the repeatable multi-repo row list
// and the legacy single dropdown, gated on executor capability (see
// getExecutorItemDisabledReason) and PR-trigger overrides (a github_pr
// trigger always uses the PR's own repository, so the picker stays a single
// disabled field even when the executor otherwise supports multi-repo).
function RepositoryPickerField({
  supportsMultiRepo,
  isPRTrigger,
  repositories,
  discoveredRepos,
  repositorySelections,
  singleRepositoryItems,
  isDirty,
  onRepositoriesChange,
}: {
  supportsMultiRepo: boolean;
  isPRTrigger: boolean;
  repositories: Repository[];
  discoveredRepos: LocalRepository[];
  repositorySelections: RepositorySelection[];
  singleRepositoryItems: Array<{ id: string; label: string }>;
  isDirty: boolean;
  onRepositoriesChange: (selections: RepositorySelection[]) => void;
}) {
  if (supportsMultiRepo && !isPRTrigger) {
    return (
      <AutomationRepositoryRows
        testId="repository-rows"
        repositories={repositories}
        discoveredRepos={discoveredRepos}
        selections={repositorySelections}
        onChange={onRepositoriesChange}
        isDirty={isDirty}
      />
    );
  }
  return (
    <SelectField
      testId="repository-selector"
      label="Repository"
      value={selectionToOptionId(repositorySelections[0] ?? { kind: "none" })}
      isDirty={isDirty}
      onChange={(v) => {
        const picked = pickSelectionFromOptionId(v, repositories, discoveredRepos);
        onRepositoriesChange(picked.kind === "none" ? [] : [picked]);
      }}
      placeholder="Auto"
      items={singleRepositoryItems}
      disabled={isPRTrigger}
      helpText={isPRTrigger ? "PR triggers always use the PR's own repository." : undefined}
    />
  );
}

function WorkflowFields({
  workflowId,
  workflowStepId,
  workflows,
  steps,
  workflowDirty,
  workflowStepDirty,
  onWorkflowChange,
  onStepChange,
}: {
  workflowId: string;
  workflowStepId: string;
  workflows: Array<{ id: string; name: string }>;
  steps: StepOption[];
  workflowDirty: boolean;
  workflowStepDirty: boolean;
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
        required
        value={workflowId}
        isDirty={workflowDirty}
        onChange={onWorkflowChange}
        placeholder="Select workflow"
        items={workflows.map((w) => ({ id: w.id, label: w.name }))}
        helpText={!hasWorkflow ? "Select a workflow to enable saving." : undefined}
      />
      <SelectField
        testId="workflow-step-selector"
        label="Workflow Step"
        required
        value={workflowStepId}
        isDirty={workflowStepDirty}
        onChange={onStepChange}
        placeholder={hasWorkflow ? "Select step" : "Pick a workflow first"}
        items={steps.map((s) => ({ id: s.id, label: s.name }))}
        disabled={!hasWorkflow}
        helpText={getWorkflowStepHelpText(workflowId, workflowStepId)}
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
  required,
  isDirty = false,
}: {
  testId?: string;
  label: string;
  value: string;
  onChange: (value: string) => void;
  placeholder: string;
  items: Array<{ id: string; label: string; disabled?: boolean; disabledReason?: string }>;
  disabled?: boolean;
  helpText?: string;
  required?: boolean;
  isDirty?: boolean;
}) {
  const invalid = required && !value;
  const helpId = testId && helpText ? `${testId}-help` : undefined;
  return (
    <div className="space-y-1.5">
      {required ? (
        <RequiredFieldLabel className="text-xs">{label}</RequiredFieldLabel>
      ) : (
        <Label className="text-xs">{label}</Label>
      )}
      <Select value={value || undefined} onValueChange={onChange} disabled={disabled}>
        <SelectTrigger
          data-testid={testId}
          className="cursor-pointer"
          aria-describedby={invalid ? helpId : undefined}
          aria-invalid={invalid ? true : undefined}
          data-settings-dirty={isDirty}
        >
          <SelectValue placeholder={placeholder} />
        </SelectTrigger>
        <SelectContent>
          {items.map((item) => (
            <SelectItem
              key={item.id}
              value={item.id}
              disabled={item.disabled}
              title={item.disabledReason}
            >
              {item.label}
            </SelectItem>
          ))}
        </SelectContent>
      </Select>
      {helpText && (
        <p id={helpId} className="text-[10px] text-muted-foreground">
          {helpText}
        </p>
      )}
    </div>
  );
}
