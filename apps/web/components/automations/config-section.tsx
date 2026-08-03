"use client";

import { useTranslation } from "react-i18next";
import { useState, useEffect, useMemo, useCallback } from "react";
import { Label } from "@kandev/ui/label";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@kandev/ui/select";
import { useAppStore } from "@/components/state-provider";
import { useSettingsData } from "@/hooks/domains/settings/use-settings-data";
import { useRepositories } from "@/hooks/domains/workspace/use-repositories";
import { discoverRepositoriesAction } from "@/app/actions/workspaces";
import { listWorkflows } from "@/lib/api";
import { listWorkflowSteps } from "@/lib/api/domains/workflow-api";
import type { ExecutorProfile, LocalRepository, Repository } from "@/lib/types/http";
import type { TriggerType } from "@/lib/types/automation";
import { getMultiRepoExecutorDisabledReason } from "@/components/task-create-dialog-multi-repo-guard";
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
  conditionType: TriggerType | null;
  dirtyFields?: {
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
};

/**
 * Stands in for "no selection" in the workflow and step pickers. Radix refuses
 * an empty SelectItem value, so clearing a field needs an id of its own; it is
 * mapped back to "" before it reaches the form.
 */
const NONE_OPTION_ID = "__none__";

const CLEAN_FIELDS = {
  workflowId: false,
  workflowStepId: false,
  agentProfileId: false,
  executorProfileId: false,
  repositorySelections: false,
};

type WorkflowOption = { id: string; name: string };

// A page opened while the backend is restarting gets a bare "Failed to fetch".
// A single attempt would leave the field empty for the rest of the session, so
// back off and try again before giving up.
const WORKFLOW_RETRY_DELAYS_MS = [500, 1500, 4000];

/**
 * Workflows for the workspace being edited, fetched into local state.
 *
 * Deliberately not the `workflows` store slot. That slot is global and both the
 * active workspace and this settings page write to it, so when the two differ
 * they race — the network log shows the two workspaces' requests alternating —
 * and whichever lands last defines the list. That produced two failures: the
 * editor offered another workspace's workflows (a name present in both, like
 * "Feature Dev", makes the wrong one look right, so an automation could be
 * saved against a workflow its workspace does not own), and filtering the
 * shared slot by workspace instead left the list empty whenever the foreign
 * fetch won.
 *
 * Owning the request here makes the list depend only on the workspace this
 * editor is for.
 */
function useWorkspaceWorkflows(workspaceId: string) {
  const [workflows, setWorkflows] = useState<WorkflowOption[]>([]);
  const [failed, setFailed] = useState(false);
  const [retryNonce, setRetryNonce] = useState(0);

  useEffect(() => {
    if (!workspaceId) {
      setWorkflows([]);
      setFailed(false);
      return;
    }
    let cancelled = false;
    let timer: ReturnType<typeof setTimeout> | undefined;

    const attempt = (index: number) => {
      listWorkflows(workspaceId, { cache: "no-store", includeHidden: true })
        .then((response) => {
          if (cancelled) return;
          setWorkflows(response.workflows.map((w) => ({ id: w.id, name: w.name })));
          setFailed(false);
        })
        .catch(() => {
          if (cancelled) return;
          if (index < WORKFLOW_RETRY_DELAYS_MS.length) {
            timer = setTimeout(() => attempt(index + 1), WORKFLOW_RETRY_DELAYS_MS[index]);
            return;
          }
          // Keep whatever was already loaded rather than blanking a usable
          // field on a flake, and say so rather than just looking empty.
          setFailed(true);
        });
    };

    // Clear before fetching, not after. Keeping the previous list across a
    // workspace change is the exact bug this hook exists to prevent: if the new
    // workspace's fetch then fails, the old workspace's workflows stay
    // selectable — and savable — under the new workspace's name.
    setWorkflows([]);
    setFailed(false);
    attempt(0);
    return () => {
      cancelled = true;
      if (timer) clearTimeout(timer);
    };
  }, [workspaceId, retryNonce]);

  const retry = useCallback(() => setRetryNonce((nonce) => nonce + 1), []);
  return { workflows, failed, retry };
}

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

function getWorkflowStepHelpText(workflowId: string): string | undefined {
  if (!workflowId) return "Select a workflow before choosing a step.";
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
  conditionType,
  dirtyFields = CLEAN_FIELDS,
  onWorkflowChange,
  onStepChange,
  onAgentProfileChange,
  onExecutorProfileChange,
  onRepositoriesChange,
}: ConfigSectionProps) {
  useSettingsData(true);
  const { repositories } = useRepositories(workspaceId, true);
  const discoveredRepos = useDiscoveredRepositories(workspaceId);

  const workflowState = useWorkspaceWorkflows(workspaceId);
  const workflows = workflowState.workflows;
  const agentProfiles = useAppStore((state) => state.agentProfiles.items);
  const executors = useAppStore((state) => state.executors.items);
  const steps = useWorkflowSteps(workflowId);
  const isPRTrigger = conditionType === "github_pr";
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
        <WorkflowFields
          workflowId={workflowId}
          workflowStepId={workflowStepId}
          workflows={workflows}
          workflowsFailed={workflowState.failed}
          onRetryWorkflows={workflowState.retry}
          steps={steps}
          workflowDirty={dirtyFields.workflowId}
          workflowStepDirty={dirtyFields.workflowStepId}
          onWorkflowChange={onWorkflowChange}
          onStepChange={onStepChange}
        />
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
  workflowsFailed,
  onRetryWorkflows,
  steps,
  workflowDirty,
  workflowStepDirty,
  onWorkflowChange,
  onStepChange,
}: {
  workflowId: string;
  workflowStepId: string;
  workflows: Array<{ id: string; name: string }>;
  workflowsFailed: boolean;
  onRetryWorkflows: () => void;
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
  const { t } = useTranslation();
  const hasWorkflow = !!workflowId;
  // Both fields are optional: an automation that only reports has no place on a
  // board, and demanding a workflow before it can be saved made every such
  // automation pick one at random. Nothing here blocks saving, so nothing here
  // says it does.
  //
  // Optional also has to mean reversible. A select listing only real workflows
  // can be set but never unset, which would strand every automation upgraded
  // from the era when a workflow was mandatory. An explicit None entry is the
  // way back; picking it clears the step too, since a step without its workflow
  // is not a selection anyone can act on.
  return (
    <>
      <SelectField
        testId="workflow-selector"
        label="Workflow"
        value={workflowId}
        isDirty={workflowDirty}
        onChange={(value) => onWorkflowChange(value === NONE_OPTION_ID ? "" : value)}
        placeholder={
          workflowsFailed
            ? t("automations:couldNotLoadWorkflows")
            : t("automations:selectWorkflowOptional")
        }
        items={[
          { id: NONE_OPTION_ID, label: t("automations:noWorkflow") },
          ...workflows.map((w) => ({ id: w.id, label: w.name })),
        ]}
      />
      {workflowsFailed && (
        <p className="text-[10px] text-destructive" data-testid="workflow-load-error">
          {t("automations:workflowLoadError")}{" "}
          <button
            type="button"
            onClick={onRetryWorkflows}
            className="cursor-pointer underline underline-offset-2"
            data-testid="workflow-retry"
          >
            {t("automations:tryAgain")}
          </button>
        </p>
      )}
      <SelectField
        testId="workflow-step-selector"
        label="Workflow Step"
        value={workflowStepId}
        isDirty={workflowStepDirty}
        onChange={(value) => onStepChange(value === NONE_OPTION_ID ? "" : value)}
        placeholder={
          hasWorkflow ? t("automations:selectStepOptional") : t("automations:pickAWorkflowFirst")
        }
        items={[
          { id: NONE_OPTION_ID, label: t("automations:noStep") },
          ...steps.map((s) => ({ id: s.id, label: s.name })),
        ]}
        disabled={!hasWorkflow}
        helpText={getWorkflowStepHelpText(workflowId)}
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
  isDirty?: boolean;
}) {
  const helpId = testId && helpText ? `${testId}-help` : undefined;
  return (
    <div className="space-y-1.5">
      <Label className="text-xs">{label}</Label>
      <Select value={value || undefined} onValueChange={onChange} disabled={disabled}>
        <SelectTrigger
          data-testid={testId}
          className="cursor-pointer"
          aria-describedby={helpId}
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
