"use client";

import { useState } from "react";
import { useAppStore } from "@/components/state-provider";
import { canSaveSandboxSelection } from "@/lib/sandbox-selection";
import { useSandboxBinding } from "./use-sandbox-binding";
import { useSandboxPolicyReads } from "./use-sandbox-policy-reads";

type Options = {
  scope: "agent" | "workflow_step" | "task";
  targetId: string;
  identity: string;
  canRead: boolean;
  canEdit: boolean;
  enabled?: boolean;
  agentProfileId?: string;
  executorId?: string;
  workflowStepId?: string;
};

function usePreviewContext(options: Options) {
  const key = JSON.stringify([options.identity, options.scope, options.targetId]);
  const [selection, setSelection] = useState({ key, executor: "", agent: "" });
  const current = selection.key === key ? selection : { key, executor: "", agent: "" };
  const executorId = options.executorId || current.executor;
  const profileId =
    options.scope === "agent" ? options.targetId : options.agentProfileId || current.agent;
  const profiles = useAppStore((state) => state.agentProfiles.items);
  return {
    executorId,
    executorContextFixed: Boolean(options.executorId),
    setExecutorId: (executor: string) => setSelection({ ...current, executor }),
    profileContext: {
      profileId,
      profiles,
      needsChoice: options.scope !== "agent" && !options.agentProfileId,
      setPreviewId: (agent: string) => setSelection({ ...current, agent }),
    },
  };
}

export function useSandboxSelectionModel(options: Options) {
  const enabled = options.enabled !== false && options.canRead && Boolean(options.targetId);
  const previewContext = usePreviewContext(options);
  const { executorId, profileContext } = previewContext;
  const executors = useAppStore((state) => state.executors.items);
  const binding = useSandboxBinding(options.scope, options.targetId, {
    enabled,
    identity: options.identity,
  });
  const context = selectionContext(
    options,
    executorId,
    profileContext.profileId,
    binding.saved?.workspace_id,
  );
  const canEdit = enabled && options.canEdit;
  const reads = useSandboxPolicyReads(
    options.identity,
    context,
    binding.draft?.revision_id ?? "",
    canEdit && !binding.loading && Boolean(profileContext.profileId),
    binding.saved?.version,
  );
  const eligible = reads.preview.data?.eligible ?? [];
  const ready = selectionReady(binding, reads.preview, canEdit, executorId);
  const canSave = canSaveSandboxSelection(binding.draft?.revision_id ?? "", eligible, ready);
  return { ...previewContext, executors, binding, reads, eligible, canSave, canEdit };
}

function selectionContext(
  options: Options,
  executorId: string,
  agentProfileId: string,
  workspaceId?: string,
) {
  return {
    executor_id: executorId,
    workspace_id: workspaceId ?? "",
    agent_profile_id: agentProfileId,
    workflow_step_id: options.scope === "workflow_step" ? options.targetId : options.workflowStepId,
    task_id: options.scope === "task" ? options.targetId : undefined,
  };
}

function selectionReady(
  binding: ReturnType<typeof useSandboxBinding>,
  preview: ReturnType<typeof useSandboxPolicyReads>["preview"],
  canEdit: boolean,
  executorId: string,
) {
  return (
    canEdit &&
    preview.isSuccess &&
    !binding.loading &&
    !binding.saving &&
    Boolean(executorId) &&
    !preview.isFetching
  );
}

export type SandboxSelectionModel = ReturnType<typeof useSandboxSelectionModel>;
