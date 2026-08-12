"use client";

import { useCallback, useState } from "react";
import { useTranslation } from "react-i18next";
import { useToast } from "@/components/toast-provider";
import { useGitOperations, type GitOperationResult } from "@/hooks/use-git-operations";
import type { RemoteContributionRelation } from "@/hooks/domains/session/remote-contribution-relation";
import type { TaskPR } from "@/lib/types/github";

export type RemoteContributionResolutionAction = "replace" | "use";

export type RemoteContributionResolutionTarget = {
  expectedRemoteHead: string;
  /** Always pass the selected repository scope, including the empty root scope. */
  repo: string;
  repositoryName?: string;
};

export type PendingRemoteContributionResolution = RemoteContributionResolutionTarget & {
  action: RemoteContributionResolutionAction;
};

export function buildRemoteContributionResolutionTarget(
  relation: RemoteContributionRelation,
  repositoryName: string | undefined,
  selectedPR: TaskPR | null | undefined,
  remoteRepositoryLabel: string,
): RemoteContributionResolutionTarget | null {
  const providerHead = relation.providerHead;
  if (!providerHead || (!relation.canReplaceRemote && !relation.canUseRemote)) return null;
  const repositoryScope = repositoryName ?? "";
  return {
    expectedRemoteHead: providerHead,
    repo: repositoryScope,
    repositoryName:
      repositoryScope ||
      (selectedPR ? `${selectedPR.owner}/${selectedPR.repo}` : remoteRepositoryLabel),
  };
}

export type RemoteContributionResolutionError = "lease_mismatch" | "dirty_worktree" | "generic";

export function classifyRemoteContributionError(
  errorCode: string | undefined,
): RemoteContributionResolutionError {
  if (errorCode === "lease_mismatch") return "lease_mismatch";
  if (errorCode === "dirty_worktree") return "dirty_worktree";
  return "generic";
}

export function remoteContributionResolutionErrorKey(
  error: RemoteContributionResolutionError,
): string {
  if (error === "lease_mismatch") return "task:remoteContributionProviderChangedAgain";
  if (error === "dirty_worktree") return "task:remoteContributionCleanWorktreeRequired";
  return "task:remoteContributionResolutionFailed";
}

export function useRemoteContributionResolution(
  sessionId: string | null | undefined,
  refreshProviderEvidence?: () => Promise<string | null>,
) {
  const { replaceRemoteContribution, useRemoteContribution, isLoading } = useGitOperations(
    sessionId ?? null,
  );
  const [pending, setPending] = useState<PendingRemoteContributionResolution | null>(null);
  const [lastResult, setLastResult] = useState<GitOperationResult | null>(null);
  const [error, setError] = useState<RemoteContributionResolutionError | null>(null);

  const begin = useCallback(
    (action: RemoteContributionResolutionAction, target: RemoteContributionResolutionTarget) => {
      setPending({ action, ...target });
      setLastResult(null);
      setError(null);
    },
    [],
  );

  const requestReplace = useCallback(
    (target: RemoteContributionResolutionTarget) => begin("replace", target),
    [begin],
  );

  const requestUse = useCallback(
    (target: RemoteContributionResolutionTarget) => begin("use", target),
    [begin],
  );

  const cancel = useCallback(() => {
    setPending(null);
    setError(null);
  }, []);

  const confirm = useCallback(async (): Promise<GitOperationResult | null> => {
    if (!pending) return null;
    try {
      const operation =
        pending.action === "replace" ? replaceRemoteContribution : useRemoteContribution;
      const result = await operation(pending.expectedRemoteHead, pending.repo);
      const resolutionError = result.success
        ? null
        : classifyRemoteContributionError(result.error_code);
      let refreshedHead: string | null = null;
      if (result.success || resolutionError === "lease_mismatch") {
        try {
          refreshedHead = (await refreshProviderEvidence?.()) ?? null;
        } catch {
          // The Git operation result remains authoritative if provider refresh fails.
        }
      }
      if (result.success) setPending(null);
      if (!result.success && resolutionError === "lease_mismatch" && refreshedHead) {
        setPending((current) =>
          current ? { ...current, expectedRemoteHead: refreshedHead } : current,
        );
      }
      setLastResult(result);
      setError(resolutionError);
      return result;
    } catch {
      setError("generic");
      return null;
    }
  }, [pending, refreshProviderEvidence, replaceRemoteContribution, useRemoteContribution]);

  return {
    pending,
    isLoading,
    lastResult,
    error,
    errorKey: error ? remoteContributionResolutionErrorKey(error) : null,
    requestReplace,
    requestUse,
    cancel,
    confirm,
  };
}

export function useRemoteContributionResolutionConfirmation(
  resolution: ReturnType<typeof useRemoteContributionResolution> | null | undefined,
) {
  const { t } = useTranslation();
  const { toast } = useToast();
  return useCallback(async () => {
    const action = resolution?.pending?.action;
    const result = await resolution?.confirm();
    if (!result?.success) return;
    toast({
      title:
        action === "replace"
          ? t("task:remoteContributionReplaced")
          : t("task:remoteContributionUsed", {
              branch: result.recovery_branch || t("task:remoteRepository"),
            }),
      variant: "success",
    });
  }, [resolution?.confirm, resolution?.pending?.action, t, toast]);
}
