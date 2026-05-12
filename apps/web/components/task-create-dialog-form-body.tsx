"use client";

import { memo, useCallback, useState } from "react";
import Link from "next/link";
import type { AgentProfileOption } from "@/lib/state/slices";
import type { WorkflowSnapshotData } from "@/lib/state/slices/kanban/types";
import { WorkflowSelectorRow } from "@/components/workflow-selector-row";
import { AgentLogo } from "@/components/agent-logo";
import type { DialogFormState } from "@/components/task-create-dialog-types";
import type { useKeyboardShortcutHandler } from "@/hooks/use-keyboard-shortcut";
import { TaskFormInputs } from "@/components/task-create-dialog-selectors";
import type { JiraTicket } from "@/lib/types/jira";
import type { LinearIssue } from "@/lib/types/linear";

type SelectorOption = {
  value: string;
  label: string;
  renderLabel: () => React.ReactNode;
};

type CreateEditSelectorsProps = {
  isTaskStarted: boolean;
  agentProfiles: AgentProfileOption[];
  agentProfilesLoading: boolean;
  agentProfileOptions: SelectorOption[];
  agentProfileId: string;
  onAgentProfileChange: (value: string) => void;
  isCreatingSession: boolean;
  executorProfileOptions: Array<{
    value: string;
    label: string;
    renderLabel?: () => React.ReactNode;
  }>;
  executorProfileId: string;
  onExecutorProfileChange: (value: string) => void;
  executorsLoading: boolean;
  AgentSelectorComponent: React.ComponentType<{
    options: SelectorOption[];
    value: string;
    onValueChange: (value: string) => void;
    disabled: boolean;
    placeholder: string;
    triggerClassName?: string;
  }>;
  ExecutorProfileSelectorComponent: React.ComponentType<{
    options: Array<{ value: string; label: string; renderLabel?: () => React.ReactNode }>;
    value: string;
    onValueChange: (value: string) => void;
    disabled: boolean;
    placeholder: string;
    triggerClassName?: string;
  }>;
  workflowAgentLocked: boolean;
};

type AgentColumnProps = Pick<
  CreateEditSelectorsProps,
  | "agentProfiles"
  | "agentProfilesLoading"
  | "agentProfileOptions"
  | "agentProfileId"
  | "onAgentProfileChange"
  | "isCreatingSession"
  | "AgentSelectorComponent"
  | "workflowAgentLocked"
>;

function AgentColumn({
  agentProfiles,
  agentProfilesLoading,
  agentProfileOptions,
  agentProfileId,
  onAgentProfileChange,
  isCreatingSession,
  AgentSelectorComponent,
  workflowAgentLocked,
}: AgentColumnProps) {
  if (agentProfiles.length === 0 && !agentProfilesLoading) {
    return (
      <div className="flex h-7 items-center justify-center gap-2 rounded-sm border border-input px-3 text-xs text-muted-foreground">
        <span>No agents found.</span>
        <Link href="/settings/agents" className="cursor-pointer text-primary hover:underline">
          Add agent
        </Link>
      </div>
    );
  }
  const placeholder = agentProfilesLoading ? "Loading agents..." : "Select agent";
  return (
    <>
      <AgentSelectorComponent
        options={agentProfileOptions}
        value={agentProfileId}
        onValueChange={onAgentProfileChange}
        placeholder={placeholder}
        disabled={agentProfilesLoading || isCreatingSession || workflowAgentLocked}
      />
      {workflowAgentLocked && (
        <p className="text-[11px] text-muted-foreground mt-1">Agent set by workflow</p>
      )}
    </>
  );
}

export const CreateEditSelectors = memo(function CreateEditSelectors(
  props: CreateEditSelectorsProps,
) {
  if (props.isTaskStarted) return null;
  const {
    executorProfileOptions,
    executorProfileId,
    onExecutorProfileChange,
    executorsLoading,
    ExecutorProfileSelectorComponent,
  } = props;

  // Branch + repo selection (and the FreshBranchToggle, which is per-task
  // branch strategy) live in the chip row above the description; this row
  // carries only agent and executor profile selectors.
  return (
    <div className="grid gap-4 grid-cols-1 sm:grid-cols-2">
      <div>
        <AgentColumn {...props} />
      </div>
      <div>
        <ExecutorProfileSelectorComponent
          options={executorProfileOptions}
          value={executorProfileId}
          onValueChange={onExecutorProfileChange}
          placeholder={executorsLoading ? "Loading profiles..." : "Select profile"}
          disabled={executorsLoading}
        />
      </div>
    </div>
  );
});

type SessionSelectorsProps = {
  agentProfileOptions: SelectorOption[];
  agentProfileId: string;
  onAgentProfileChange: (value: string) => void;
  agentProfilesLoading: boolean;
  isCreatingSession: boolean;
  executorProfileOptions: Array<{
    value: string;
    label: string;
    renderLabel?: () => React.ReactNode;
  }>;
  executorProfileId: string;
  onExecutorProfileChange: (value: string) => void;
  executorsLoading: boolean;
  AgentSelectorComponent: React.ComponentType<{
    options: SelectorOption[];
    value: string;
    onValueChange: (value: string) => void;
    disabled: boolean;
    placeholder: string;
    triggerClassName?: string;
  }>;
  ExecutorProfileSelectorComponent: React.ComponentType<{
    options: Array<{ value: string; label: string; renderLabel?: () => React.ReactNode }>;
    value: string;
    onValueChange: (value: string) => void;
    disabled: boolean;
    placeholder: string;
    triggerClassName?: string;
  }>;
};

export const SessionSelectors = memo(function SessionSelectors({
  agentProfileOptions,
  agentProfileId,
  onAgentProfileChange,
  agentProfilesLoading,
  isCreatingSession,
  executorProfileOptions,
  executorProfileId,
  onExecutorProfileChange,
  executorsLoading,
  AgentSelectorComponent,
  ExecutorProfileSelectorComponent,
}: SessionSelectorsProps) {
  return (
    <div className="grid grid-cols-1 sm:grid-cols-2 gap-3">
      <AgentSelectorComponent
        options={agentProfileOptions}
        value={agentProfileId}
        onValueChange={onAgentProfileChange}
        placeholder={agentProfilesLoading ? "Loading agent profiles..." : "Select agent profile"}
        disabled={agentProfilesLoading || isCreatingSession}
      />
      <ExecutorProfileSelectorComponent
        options={executorProfileOptions}
        value={executorProfileId}
        onValueChange={onExecutorProfileChange}
        placeholder={executorsLoading ? "Loading profiles..." : "Select profile"}
        disabled={executorsLoading || isCreatingSession}
      />
    </div>
  );
});

type WorkflowSectionProps = {
  isCreateMode: boolean;
  isTaskStarted: boolean;
  workflows: Array<{
    id: string;
    name: string;
    agent_profile_id?: string;
    hidden?: boolean;
    [key: string]: unknown;
  }>;
  snapshots: Record<string, WorkflowSnapshotData>;
  effectiveWorkflowId: string | null;
  onWorkflowChange: (value: string) => void;
  agentProfiles: AgentProfileOption[];
  /**
   * When true the picker is hidden entirely. Used by feature wrappers
   * (Improve Kandev) where the workflow is enforced and the user must not be
   * able to switch to a different one. The wrapper is responsible for
   * surfacing the workflow elsewhere (e.g. a steps preview).
   */
  workflowLocked?: boolean;
};

export const WorkflowSection = memo(function WorkflowSection({
  isCreateMode,
  isTaskStarted,
  workflows: allWorkflows,
  snapshots,
  effectiveWorkflowId,
  onWorkflowChange,
  agentProfiles,
  workflowLocked,
}: WorkflowSectionProps) {
  const [lastUsedWorkflowId, setLastUsedWorkflowId] = useState<string | null>(null);

  // Hidden workflows (e.g. improve-kandev) are excluded from the picker; they
  // remain reachable via their dedicated entry point.
  const workflows = allWorkflows.filter((w) => !w.hidden);

  const handleWorkflowChange = useCallback(
    (workflowId: string) => {
      setLastUsedWorkflowId(workflowId);
      onWorkflowChange(workflowId);
    },
    [onWorkflowChange],
  );

  if (!isCreateMode || isTaskStarted) return null;
  if (workflowLocked) return null;

  if (!effectiveWorkflowId || workflows.length > 1) {
    return (
      <WorkflowSelectorRow
        workflows={workflows}
        snapshots={snapshots}
        selectedWorkflowId={effectiveWorkflowId ?? null}
        onWorkflowChange={handleWorkflowChange}
        lastUsedWorkflowId={lastUsedWorkflowId}
        agentProfiles={agentProfiles}
      />
    );
  }

  // Single selected workflow — show agent override info if any overrides exist
  if (workflows.length === 1) {
    const singleWorkflow = workflows[0];
    if (!singleWorkflow) return null;
    const snapshot = snapshots[singleWorkflow.id];
    const workflowProfile = singleWorkflow.agent_profile_id
      ? agentProfiles.find((p) => p.id === singleWorkflow.agent_profile_id)
      : null;
    const stepsWithOverrides = (snapshot?.steps ?? [])
      .filter((s) => s.agent_profile_id)
      .map((s) => ({
        name: s.title,
        profile: agentProfiles.find((p) => p.id === s.agent_profile_id),
      }))
      .filter((s) => s.profile);
    if (!workflowProfile && stepsWithOverrides.length === 0) return null;
    return (
      <div
        className="flex flex-wrap items-center gap-2 text-xs text-muted-foreground"
        data-testid="workflow-override-info"
      >
        {workflowProfile && (
          <span className="flex items-center gap-1">
            <AgentLogo agentName={workflowProfile.agent_name} size={14} className="shrink-0" />
            <span>{workflowProfile.label}</span>
          </span>
        )}
        {stepsWithOverrides.map((s) => (
          <span key={s.name} className="flex items-center gap-1">
            <span className="text-muted-foreground/50">{s.name}:</span>
            <AgentLogo agentName={s.profile!.agent_name} size={14} className="shrink-0" />
            <span>{s.profile!.label}</span>
          </span>
        ))}
      </div>
    );
  }

  return null;
});

export type DialogPromptSectionProps = {
  isSessionMode: boolean;
  isTaskStarted: boolean;
  isPassthroughProfile: boolean;
  initialDescription: string;
  hasDescription: boolean;
  fs: DialogFormState;
  handleKeyDown: ReturnType<typeof useKeyboardShortcutHandler>;
  enhance?: { onEnhance: () => void; isLoading: boolean; isConfigured: boolean };
  workspaceId?: string | null;
  onJiraImport?: (ticket: JiraTicket) => void;
  onLinearImport?: (issue: LinearIssue) => void;
  /** Extension slot rendered below the description textarea (e.g. log-capture toggle). */
  extraFormSlot?: React.ReactNode;
  /** Optional override for the description textarea placeholder. */
  descriptionPlaceholder?: string;
  /** Optional slot rendered above the description textarea (e.g. a tab toggle). */
  aboveDescriptionSlot?: React.ReactNode;
  /**
   * Whether the description textarea should grab focus on mount. Defaults to
   * `!isTaskStarted`. Callers that render a task-name input above the
   * description should pass `false` so the name field wins focus.
   */
  autoFocusDescription?: boolean;
};

// importBindings collapses the optional Jira/Linear import callbacks into the
// shape TaskFormInputs expects, dropping integrations that aren't applicable
// (session mode, started tasks, or no callback wired). Keeps DialogPromptSection
// below the cyclomatic-complexity bar.
function importBindings<T>(
  enabled: boolean,
  workspaceId: string | null,
  isPassthroughProfile: boolean,
  onImport: ((value: T) => void) | undefined,
) {
  if (!enabled || !onImport) return undefined;
  return { workspaceId, disabled: isPassthroughProfile, onImport };
}

export function DialogPromptSection({
  isSessionMode,
  isTaskStarted,
  isPassthroughProfile,
  initialDescription,
  hasDescription,
  fs,
  handleKeyDown,
  enhance,
  workspaceId,
  onJiraImport,
  onLinearImport,
  extraFormSlot,
  descriptionPlaceholder,
  aboveDescriptionSlot,
  autoFocusDescription,
}: DialogPromptSectionProps) {
  const importsEnabled = !isSessionMode && !isTaskStarted;
  const ws = workspaceId ?? null;
  const placeholder = isPassthroughProfile
    ? "Passthrough mode — prompt not supported"
    : descriptionPlaceholder;
  const shouldAutoFocus = autoFocusDescription ?? !isTaskStarted;
  return (
    <>
      {aboveDescriptionSlot}
      <TaskFormInputs
        key={fs.openCycle}
        isSessionMode={isSessionMode}
        autoFocus={shouldAutoFocus}
        initialDescription={initialDescription}
        onDescriptionChange={fs.setHasDescription}
        onKeyDown={handleKeyDown}
        descriptionValueRef={fs.descriptionInputRef}
        disabled={isTaskStarted || isPassthroughProfile}
        placeholder={placeholder}
        onEnhancePrompt={enhance?.onEnhance}
        isEnhancingPrompt={enhance?.isLoading}
        isUtilityConfigured={enhance?.isConfigured}
        jiraImport={importBindings(importsEnabled, ws, isPassthroughProfile, onJiraImport)}
        linearImport={importBindings(importsEnabled, ws, isPassthroughProfile, onLinearImport)}
      />
      {extraFormSlot}
      {isPassthroughProfile && hasDescription && (
        <p className="text-xs text-amber-500">Prompt ignored — passthrough mode active</p>
      )}
    </>
  );
}
