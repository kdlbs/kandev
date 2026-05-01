"use client";

import type { AgentProfileOption } from "@/lib/state/slices";
import type { DialogFormState } from "@/components/task-create-dialog-types";
import { CreateEditSelectors } from "@/components/task-create-dialog-form-body";
import { AgentSelector, ExecutorProfileSelector } from "@/components/task-create-dialog-selectors";
import { useAgentProfileOptions } from "@/components/task-create-dialog-options";

type CreateModeSelectorsProps = {
  isTaskStarted: boolean;
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
  onAgentProfileChange: (v: string) => void;
  onExecutorProfileChange: (v: string) => void;
  workflowAgentLocked: boolean;
};

/**
 * Create/edit-mode form body section: agent + executor profile selectors.
 * Repo, branch and fresh-branch toggle live in the chip row above the
 * description (RepoChipsRow).
 */
export function CreateModeSelectors(props: CreateModeSelectorsProps) {
  return (
    <CreateEditSelectors
      isTaskStarted={props.isTaskStarted}
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
      workflowAgentLocked={props.workflowAgentLocked}
      AgentSelectorComponent={AgentSelector}
      ExecutorProfileSelectorComponent={ExecutorProfileSelector}
    />
  );
}
