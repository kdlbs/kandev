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

type SelectorOption = {
  value: string;
  label: string;
  renderLabel: () => React.ReactNode;
};

type CreateEditSelectorsProps = {
  isTaskStarted: boolean;
  hasRepositorySelection: boolean;
  branchOptions: SelectorOption[];
  branch: string;
  onBranchChange: (value: string) => void;
  branchesLoading: boolean;
  localBranchesLoading: boolean;
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
  BranchSelectorComponent: React.ComponentType<{
    options: SelectorOption[];
    value: string;
    onValueChange: (value: string) => void;
    disabled: boolean;
    placeholder: string;
    searchPlaceholder: string;
    emptyMessage: string;
  }>;
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
  isLocalExecutor: boolean;
  useGitHubUrl: boolean;
  workflowAgentLocked: boolean;
};

export const CreateEditSelectors = memo(function CreateEditSelectors({
  isTaskStarted,
  hasRepositorySelection,
  branchOptions,
  branch,
  onBranchChange,
  branchesLoading,
  localBranchesLoading,
  agentProfiles,
  agentProfilesLoading,
  agentProfileOptions,
  agentProfileId,
  onAgentProfileChange,
  isCreatingSession,
  executorProfileOptions,
  executorProfileId,
  onExecutorProfileChange,
  executorsLoading,
  isLocalExecutor,
  useGitHubUrl,
  workflowAgentLocked,
  BranchSelectorComponent,
  AgentSelectorComponent,
  ExecutorProfileSelectorComponent,
}: CreateEditSelectorsProps) {
  if (isTaskStarted) return null;

  const agentLockedByWorkflow = workflowAgentLocked;

  const isLocalWithoutGitHubUrl = isLocalExecutor && !useGitHubUrl;

  const branchPlaceholder = (() => {
    if (isLocalWithoutGitHubUrl) return "Uses current branch";
    if (!hasRepositorySelection) return "Select repository first";
    if (branchesLoading || localBranchesLoading) return "Loading branches...";
    return branchOptions.length > 0 ? "Select branch" : "No branches found";
  })();

  const branchDisabled =
    isLocalWithoutGitHubUrl ||
    !hasRepositorySelection ||
    branchesLoading ||
    localBranchesLoading ||
    branchOptions.length === 0;

  const agentPlaceholder = (() => {
    if (agentProfilesLoading) return "Loading agents...";
    if (agentProfiles.length === 0) return "No agents available";
    return "Select agent";
  })();

  return (
    <div className="grid gap-4 grid-cols-1 sm:grid-cols-3">
      <div>
        <BranchSelectorComponent
          options={branchOptions}
          value={branch}
          onValueChange={onBranchChange}
          placeholder={branchPlaceholder}
          searchPlaceholder="Search branches..."
          emptyMessage="No branch found."
          disabled={branchDisabled}
        />
      </div>
      <div>
        {agentProfiles.length === 0 && !agentProfilesLoading ? (
          <div className="flex h-7 items-center justify-center gap-2 rounded-sm border border-input px-3 text-xs text-muted-foreground">
            <span>No agents found.</span>
            <Link href="/settings/agents" className="text-primary hover:underline">
              Add agent
            </Link>
          </div>
        ) : (
          <>
            <AgentSelectorComponent
              options={agentProfileOptions}
              value={agentProfileId}
              onValueChange={onAgentProfileChange}
              placeholder={agentPlaceholder}
              disabled={agentProfilesLoading || isCreatingSession || agentLockedByWorkflow}
            />
            {agentLockedByWorkflow && (
              <p className="text-[11px] text-muted-foreground mt-1">Agent set by workflow</p>
            )}
          </>
        )}
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
  workflows: Array<{ id: string; name: string; agent_profile_id?: string; [key: string]: unknown }>;
  snapshots: Record<string, WorkflowSnapshotData>;
  effectiveWorkflowId: string | null;
  onWorkflowChange: (value: string) => void;
  agentProfiles: AgentProfileOption[];
};

export const WorkflowSection = memo(function WorkflowSection({
  isCreateMode,
  isTaskStarted,
  workflows,
  snapshots,
  effectiveWorkflowId,
  onWorkflowChange,
  agentProfiles,
}: WorkflowSectionProps) {
  const [lastUsedWorkflowId, setLastUsedWorkflowId] = useState<string | null>(null);

  const handleWorkflowChange = useCallback(
    (workflowId: string) => {
      setLastUsedWorkflowId(workflowId);
      onWorkflowChange(workflowId);
    },
    [onWorkflowChange],
  );

  if (!isCreateMode || isTaskStarted) return null;

  // Single workflow — show agent override info if any overrides exist
  if (workflows.length <= 1) {
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
};

export function DialogPromptSection({
  isSessionMode,
  isTaskStarted,
  isPassthroughProfile,
  initialDescription,
  hasDescription,
  fs,
  handleKeyDown,
  enhance,
}: DialogPromptSectionProps) {
  return (
    <>
      <TaskFormInputs
        key={fs.openCycle}
        isSessionMode={isSessionMode}
        autoFocus={!isTaskStarted}
        initialDescription={initialDescription}
        onDescriptionChange={fs.setHasDescription}
        onKeyDown={handleKeyDown}
        descriptionValueRef={fs.descriptionInputRef}
        disabled={isTaskStarted || isPassthroughProfile}
        placeholder={isPassthroughProfile ? "Passthrough mode — prompt not supported" : undefined}
        onEnhancePrompt={enhance?.onEnhance}
        isEnhancingPrompt={enhance?.isLoading}
        isUtilityConfigured={enhance?.isConfigured}
      />
      {isPassthroughProfile && hasDescription && (
        <p className="text-xs text-amber-500">Prompt ignored — passthrough mode active</p>
      )}
    </>
  );
}
