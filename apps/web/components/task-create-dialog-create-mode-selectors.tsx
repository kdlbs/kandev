"use client";

import type { AgentProfileOption } from "@/lib/state/slices";
import type { Repository } from "@/lib/types/http";
import type { DialogFormState } from "@/components/task-create-dialog-types";
import { CreateEditSelectors } from "@/components/task-create-dialog-form-body";
import {
  AgentSelector,
  BranchSelector,
  ExecutorProfileSelector,
} from "@/components/task-create-dialog-selectors";
import {
  useAgentProfileOptions,
  useBranchOptions,
} from "@/components/task-create-dialog-options";
import { ExtraRepositoryRows } from "@/components/task-create-dialog-extra-repos";

type CreateModeSelectorsProps = {
  isTaskStarted: boolean;
  hasRepositorySelection: boolean;
  branchOptions: ReturnType<typeof useBranchOptions>;
  branchesLoading: boolean;
  agentProfileOptions: ReturnType<typeof useAgentProfileOptions>;
  executorProfileOptions: Array<{
    value: string;
    label: string;
    renderLabel?: () => React.ReactNode;
  }>;
  agentProfiles: AgentProfileOption[];
  agentProfilesLoading: boolean;
  executorsLoading: boolean;
  isCreatingSession: boolean;
  fs: DialogFormState;
  onBranchChange: (v: string) => void;
  onAgentProfileChange: (v: string) => void;
  onExecutorProfileChange: (v: string) => void;
  isLocalExecutor: boolean;
  workflowAgentLocked: boolean;
  repositories: Repository[];
};

/**
 * Create/edit-mode form body section: branch + agent + executor selectors,
 * followed by the multi-repo extra-repository rows. Extracted from
 * task-create-dialog.tsx to keep the parent file under the lint line cap.
 */
export function CreateModeSelectors(props: CreateModeSelectorsProps) {
  return (
    <>
      <CreateEditSelectors
        isTaskStarted={props.isTaskStarted}
        hasRepositorySelection={props.hasRepositorySelection}
        branchOptions={props.branchOptions}
        branch={props.fs.branch}
        onBranchChange={props.onBranchChange}
        branchesLoading={props.branchesLoading}
        localBranchesLoading={props.fs.localBranchesLoading}
        agentProfiles={props.agentProfiles}
        agentProfilesLoading={props.agentProfilesLoading}
        agentProfileOptions={props.agentProfileOptions}
        agentProfileId={props.fs.agentProfileId}
        onAgentProfileChange={props.onAgentProfileChange}
        isCreatingSession={props.isCreatingSession}
        executorProfileOptions={props.executorProfileOptions}
        executorProfileId={props.fs.executorProfileId}
        onExecutorProfileChange={props.onExecutorProfileChange}
        executorsLoading={props.executorsLoading}
        isLocalExecutor={props.isLocalExecutor}
        useGitHubUrl={props.fs.useGitHubUrl}
        workflowAgentLocked={props.workflowAgentLocked}
        BranchSelectorComponent={BranchSelector}
        AgentSelectorComponent={AgentSelector}
        ExecutorProfileSelectorComponent={ExecutorProfileSelector}
      />
      <ExtraRepositoryRows
        fs={props.fs}
        repositories={props.repositories}
        isTaskStarted={props.isTaskStarted}
        primarySelected={props.hasRepositorySelection}
      />
    </>
  );
}
