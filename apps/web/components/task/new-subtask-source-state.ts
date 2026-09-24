"use client";

import { useMemo } from "react";
import { useTranslation } from "react-i18next";
import type { ExecutorType } from "@/lib/types/http";
import type { DialogFormState } from "@/components/task-create-dialog-types";
import { resolveRepositorySelections } from "@/components/task-create-dialog-repositories-state";
import {
  deriveExecutorSourcePolicy,
  executorSourceIncompatibilityReasonKey,
  executorSourcePolicyReasonKey,
  type ExecutorSourcePolicy,
} from "@/components/task-create-dialog-executor-source-policy";
import {
  remoteOriginSelectionIsCompatible,
  useSelectedRemoteOriginInspection,
} from "@/components/task-create-dialog-remote-origin-inspection";
import type { RepositoryCloneSourceState } from "@/hooks/domains/repositories/use-repository-clone-source";

type SelectedSource = ReturnType<typeof resolveRepositorySelections>[number];

export type SubtaskExecutorSourceState = {
  executorSourcePolicy: ExecutorSourcePolicy;
  executorSourceNotice: string | null;
  folderDisabledReason?: string;
  sourcePolicyReason: string | null;
  remoteOriginStates: Record<string, RepositoryCloneSourceState>;
  refreshRemoteOrigins: () => void;
};

export function useSubtaskExecutorSourceState({
  fs,
  workspaceId,
  executorType,
  selectedSources,
}: {
  fs: DialogFormState;
  workspaceId: string | null;
  executorType: ExecutorType | string | undefined;
  selectedSources: SelectedSource[];
}): SubtaskExecutorSourceState {
  const { t } = useTranslation();
  const executorSourceNotice = fs.folderOnlyExecutorNotice
    ? t("task:folderOnlyExecutorSwitched")
    : null;
  const sourcePolicy = useMemo(
    () => deriveExecutorSourcePolicy({ executorType, counts: sourceCounts(selectedSources) }),
    [executorType, selectedSources],
  );
  const remoteOriginInspection = useSelectedRemoteOriginInspection(fs, workspaceId, executorType);
  const pending =
    remoteOriginInspection.enabled &&
    remoteOriginInspection.candidates.some((candidate) => {
      const state = remoteOriginInspection.states[candidate.key];
      return !state || state.status === "checking";
    });
  const invalid =
    remoteOriginInspection.enabled &&
    remoteOriginInspection.candidates.some((candidate) => {
      const selection = selectedSources.find(
        (item): item is Extract<SelectedSource, { kind: "local" }> =>
          item.kind === "local" && item.key === candidate.key,
      );
      return (
        !selection ||
        !remoteOriginSelectionIsCompatible(selection, remoteOriginInspection.states[candidate.key])
      );
    });
  const executorSourcePolicy = useMemo(
    () =>
      invalid
        ? {
            ...sourcePolicy,
            incompatible: true,
            incompatibleReason: "repository_origin_unavailable" as const,
          }
        : sourcePolicy,
    [invalid, sourcePolicy],
  );
  const folderPolicyReasonKey = executorSourcePolicyReasonKey(
    executorSourcePolicy.folderDisabledReason,
  );
  const sourcePolicyReasonKey = executorSourceIncompatibilityReasonKey(
    executorSourcePolicy.incompatibleReason,
  );
  const folderDisabledReason = folderPolicyReasonKey ? t(folderPolicyReasonKey) : undefined;
  const sourcePolicyReason = resolveSourcePolicyReason(t, pending, sourcePolicyReasonKey);
  return {
    executorSourcePolicy,
    executorSourceNotice,
    folderDisabledReason,
    sourcePolicyReason,
    remoteOriginStates: remoteOriginInspection.states,
    refreshRemoteOrigins: remoteOriginInspection.refresh,
  };
}

function resolveSourcePolicyReason(
  t: (key: string) => string,
  pending: boolean,
  reasonKey: string | undefined,
): string | null {
  if (pending) return t("task:checkingRepositoryOrigin");
  if (!reasonKey) return null;
  return t(reasonKey);
}

function sourceCounts(selections: SelectedSource[]) {
  return {
    sourceCount: selections.length,
    repositoryCount: selections.filter(isRepositorySource).length,
    folderCount: selections.filter((selection) => selection.kind === "folder").length,
    localRepositoryCount: selections.filter((selection) => selection.kind === "local").length,
    remoteOriginRepositoryCount: selections.filter(
      (selection) => selection.kind === "local" && selection.checkoutSource === "remote_origin",
    ).length,
  };
}

function isRepositorySource(selection: SelectedSource): boolean {
  return selection.kind !== "folder";
}
