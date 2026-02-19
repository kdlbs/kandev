"use client";

import { memo } from "react";
import Link from "next/link";
import type { AgentProfileOption } from "@/lib/state/slices";
import type { WorkflowSnapshotData } from "@/lib/state/slices/kanban/types";
import { WorkflowSelectorRow } from "@/components/workflow-selector-row";

type SelectorOption = {
  value: string;
  label: string;
  renderLabel: () => React.ReactNode;
};

type CreateEditSelectorsProps = {
  isTaskStarted: boolean;
  hasRepositorySelection: boolean;
  repositoryId: string;
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
  executorOptions: SelectorOption[];
  executorId: string;
  onExecutorChange: (value: string) => void;
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
  ExecutorSelectorComponent: React.ComponentType<{
    options: Array<{ value: string; label: string; renderLabel?: () => React.ReactNode }>;
    value: string;
    onValueChange: (value: string) => void;
    disabled: boolean;
    placeholder: string;
    triggerClassName?: string;
  }>;
};

export const CreateEditSelectors = memo(function CreateEditSelectors({
  isTaskStarted,
  hasRepositorySelection,
  repositoryId,
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
  executorOptions,
  executorId,
  onExecutorChange,
  executorsLoading,
  BranchSelectorComponent,
  AgentSelectorComponent,
  ExecutorSelectorComponent,
}: CreateEditSelectorsProps) {
  if (isTaskStarted) return null;

  const branchPlaceholder = (() => {
    if (!hasRepositorySelection) return "Select repository first";
    if (repositoryId) {
      return branchesLoading ? "Loading branches..." : "Select branch";
    }
    if (localBranchesLoading) return "Loading branches...";
    return branchOptions.length > 0 ? "Select branch" : "No branches found";
  })();

  const branchDisabled =
    !hasRepositorySelection ||
    (repositoryId ? branchesLoading : localBranchesLoading || branchOptions.length === 0);

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
          <AgentSelectorComponent
            options={agentProfileOptions}
            value={agentProfileId}
            onValueChange={onAgentProfileChange}
            placeholder={agentPlaceholder}
            disabled={agentProfilesLoading || isCreatingSession}
          />
        )}
      </div>
      <div>
        <ExecutorSelectorComponent
          options={executorOptions}
          value={executorId}
          onValueChange={onExecutorChange}
          placeholder={executorsLoading ? "Loading executors..." : "Select executor"}
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
  executorOptions: SelectorOption[];
  executorId: string;
  onExecutorChange: (value: string) => void;
  executorsLoading: boolean;
  AgentSelectorComponent: React.ComponentType<{
    options: SelectorOption[];
    value: string;
    onValueChange: (value: string) => void;
    disabled: boolean;
    placeholder: string;
    triggerClassName?: string;
  }>;
  ExecutorSelectorComponent: React.ComponentType<{
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
  executorOptions,
  executorId,
  onExecutorChange,
  executorsLoading,
  AgentSelectorComponent,
  ExecutorSelectorComponent,
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
      <ExecutorSelectorComponent
        options={executorOptions}
        value={executorId}
        onValueChange={onExecutorChange}
        placeholder={executorsLoading ? "Loading executors..." : "Select executor"}
        disabled={executorsLoading || isCreatingSession}
      />
    </div>
  );
});

type WorkflowSectionProps = {
  isCreateMode: boolean;
  isTaskStarted: boolean;
  workflows: Array<{ id: string; name: string; [key: string]: unknown }>;
  snapshots: Record<string, WorkflowSnapshotData>;
  effectiveWorkflowId: string | null;
  onWorkflowChange: (value: string) => void;
};

export const WorkflowSection = memo(function WorkflowSection({
  isCreateMode,
  isTaskStarted,
  workflows,
  snapshots,
  effectiveWorkflowId,
  onWorkflowChange,
}: WorkflowSectionProps) {
  if (!isCreateMode || workflows.length <= 1 || isTaskStarted) return null;
  return (
    <WorkflowSelectorRow
      workflows={workflows}
      snapshots={snapshots}
      selectedWorkflowId={effectiveWorkflowId ?? null}
      onWorkflowChange={onWorkflowChange}
    />
  );
});
