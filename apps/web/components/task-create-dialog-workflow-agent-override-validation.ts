import type { AgentProfileOption } from "@/lib/state/slices";
import type { WorkflowSnapshotData } from "@/lib/state/slices/kanban/types";
import { t } from "@/lib/i18n";
import {
  buildWorkflowAgentOverrideRows,
  type WorkflowAgentOverrideOption,
  type WorkflowAgentOverrideRow,
} from "@/components/task-create-dialog-workflow-agent-overrides";

export type WorkflowAgentOverrideValidation = {
  rows: WorkflowAgentOverrideRow[];
  options: WorkflowAgentOverrideOption[];
  loading: boolean;
  error: boolean;
  invalid: boolean;
  blockedReason?: string;
};

function resolveSnapshotState(args: {
  effectiveWorkflowId: string | null;
  snapshots: Record<string, WorkflowSnapshotData>;
  workspaceSnapshotRead?: { workspaceId?: string | null; error?: unknown };
  workspaceId: string | null | undefined;
}) {
  const snapshot = args.effectiveWorkflowId ? args.snapshots[args.effectiveWorkflowId] : undefined;
  const missingSnapshot = Boolean(args.effectiveWorkflowId && !snapshot);
  const workspaceSnapshotRead = args.workspaceSnapshotRead ?? {};
  const readBelongsToWorkspace =
    !workspaceSnapshotRead.workspaceId || workspaceSnapshotRead.workspaceId === args.workspaceId;
  const error = Boolean(missingSnapshot && workspaceSnapshotRead.error && readBelongsToWorkspace);
  return {
    snapshot,
    error,
    loading: Boolean(missingSnapshot && !error) || Boolean(snapshot?.isPlaceholder),
  };
}

function blockedReasonForValidation(args: {
  isCreateMode: boolean;
  hasSelectedOverride: boolean;
  error: boolean;
  loading: boolean;
  invalid: boolean;
}) {
  if (!args.isCreateMode) return undefined;
  if (!args.hasSelectedOverride) return undefined;
  if (args.error) return t("task:workflowAgentsLoadError");
  if (args.loading) return t("task:workflowAgentsLoading");
  return args.invalid ? t("task:workflowAgentsUnavailable") : undefined;
}

export function buildWorkflowAgentOverrideValidation(args: {
  effectiveWorkflowId: string | null | undefined;
  snapshots: Record<string, WorkflowSnapshotData>;
  workspaceSnapshotRead?: {
    workspaceId?: string | null;
    error?: unknown;
  };
  workspaceId: string | null | undefined;
  profiles: readonly AgentProfileOption[];
  replacementOptions: readonly WorkflowAgentOverrideOption[];
  overrides: Readonly<Record<string, string>>;
  isCreateMode: boolean;
}): WorkflowAgentOverrideValidation {
  const effectiveWorkflowId = args.effectiveWorkflowId ?? null;
  const snapshotState = resolveSnapshotState({
    effectiveWorkflowId,
    snapshots: args.snapshots,
    workspaceSnapshotRead: args.workspaceSnapshotRead,
    workspaceId: args.workspaceId,
  });
  const rows = buildWorkflowAgentOverrideRows({
    steps: snapshotState.snapshot?.steps ?? [],
    profiles: args.profiles,
    replacementOptions: args.replacementOptions,
    overrides: args.overrides,
  });
  const hasSelectedOverride = Object.values(args.overrides ?? {}).some(
    (replacementProfileId) => replacementProfileId.trim() !== "",
  );
  const invalid = !snapshotState.loading && rows.some((row) => !row.replacementAvailable);
  return {
    rows,
    options: args.replacementOptions as WorkflowAgentOverrideOption[],
    loading: snapshotState.loading,
    error: snapshotState.error,
    invalid,
    blockedReason: blockedReasonForValidation({
      isCreateMode: args.isCreateMode,
      hasSelectedOverride,
      error: snapshotState.error,
      loading: snapshotState.loading,
      invalid,
    }),
  };
}
