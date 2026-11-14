"use client";

import { useCallback } from "react";
import { createTask as createTaskNative, type createTask } from "@/lib/api";
import { ApiError } from "@/lib/api/client";
import { dirtyFilesFromApiError } from "@/components/task-create-dialog-fresh-branch-consent";
import {
  finishTaskCreateNavigation,
  shouldNavigateAfterTaskCreate,
} from "@/components/task-create-dialog-navigation";
import type { CreateTaskResponse, Task } from "@/lib/types/http";
import type { TaskCreationReceipt } from "@/lib/task-creation-receipt";
import type { TaskCreateRecovery } from "@/components/task-create-dialog-recovery";

type CreateTaskParams = Parameters<typeof createTask>[0];

export type NativeSendResult =
  | { kind: "response"; response: CreateTaskResponse }
  | { kind: "uncertain" }
  | { kind: "cancelled" }
  | { kind: "rejected"; error: unknown }
  | { kind: "stale" };

export function isStaleBranchPolicyError(error: unknown): boolean {
  return (
    error instanceof ApiError && error.status === 400 && error.errorCode === "branch_policy_stale"
  );
}

function isDefinitiveCreateRejection(error: unknown, receipt: TaskCreationReceipt): boolean {
  if (!(error instanceof ApiError)) return false;
  if (receipt.everUncertain || receipt.taskId) return false;
  const body = error.body as { creation_rejected?: unknown } | null;
  return body?.creation_rejected === true;
}

type NativeCreateError =
  | { kind: "fresh-branch"; dirtyFiles: string[] }
  | { kind: "stale-policy"; error: unknown }
  | { kind: "definitive"; error: unknown }
  | { kind: "uncertain" };

function classifyNativeCreateError(
  error: unknown,
  candidate: TaskCreationReceipt,
): NativeCreateError {
  const dirtyFiles = dirtyFilesFromApiError(error);
  if (dirtyFiles) return { kind: "fresh-branch", dirtyFiles };
  if (isStaleBranchPolicyError(error)) return { kind: "stale-policy", error };
  if (isDefinitiveCreateRejection(error, candidate)) return { kind: "definitive", error };
  return { kind: "uncertain" };
}

type FreshBranchRetry =
  | { kind: "retry"; candidate: TaskCreationReceipt }
  | { kind: "cancelled" }
  | { kind: "stale" };

async function retryWithFreshBranchConsent(args: {
  recovery: TaskCreateRecovery;
  candidate: TaskCreationReceipt;
  dirtyFiles: string[];
  generation: number;
  confirmForDirtyFiles: (dirtyFiles: string[]) => Promise<string[] | null>;
}): Promise<FreshBranchRetry> {
  const consented = await args.confirmForDirtyFiles(args.dirtyFiles);
  if (!args.recovery.isOwnerCurrent(args.generation)) {
    args.recovery.markUncertain(args.candidate);
    return { kind: "stale" };
  }
  if (consented === null) {
    args.recovery.markUncertain(args.candidate);
    return { kind: "cancelled" };
  }
  return {
    kind: "retry",
    candidate: args.recovery.updateConsent(args.candidate, consented),
  };
}

async function resolveStalePolicyFailure(args: {
  recovery: TaskCreateRecovery;
  candidate: TaskCreationReceipt;
  error: unknown;
  generation: number;
  refreshStaleBranchPolicies: (error: unknown) => Promise<unknown>;
}): Promise<NativeSendResult> {
  if (isDefinitiveCreateRejection(args.error, args.candidate)) {
    args.recovery.retire(args.candidate);
    return { kind: "rejected", error: args.error };
  }
  await args.refreshStaleBranchPolicies(args.error);
  if (!args.recovery.isOwnerCurrent(args.generation)) {
    args.recovery.markUncertain(args.candidate);
    return { kind: "stale" };
  }
  args.recovery.markUncertain(args.candidate);
  return { kind: "uncertain" };
}

export function useNativeCreateTransport(args: {
  recovery: TaskCreateRecovery;
  isFreshBranchActive: boolean;
  confirmForDirtyFiles: (dirtyFiles: string[]) => Promise<string[] | null>;
  refreshStaleBranchPolicies: (error: unknown) => Promise<unknown>;
}) {
  const { recovery, isFreshBranchActive, confirmForDirtyFiles, refreshStaleBranchPolicies } = args;
  const send = useCallback(
    async (
      candidate: TaskCreationReceipt,
      reconsented: boolean,
      generation: number,
    ): Promise<NativeSendResult> => {
      if (!recovery.isOwnerCurrent(generation)) return { kind: "stale" };
      try {
        const response = await createTaskNative(candidate.payload);
        return { kind: "response", response };
      } catch (error) {
        const classified = classifyNativeCreateError(error, candidate);
        if (classified.kind === "fresh-branch" && !reconsented && isFreshBranchActive) {
          const retry = await retryWithFreshBranchConsent({
            recovery,
            candidate,
            dirtyFiles: classified.dirtyFiles,
            generation,
            confirmForDirtyFiles,
          });
          if (retry.kind !== "retry") return retry;
          return send(retry.candidate, true, generation);
        }
        if (classified.kind === "stale-policy") {
          return resolveStalePolicyFailure({
            recovery,
            candidate,
            error: classified.error,
            generation,
            refreshStaleBranchPolicies,
          });
        }
        if (classified.kind === "definitive") {
          recovery.retire(candidate);
          return { kind: "rejected", error: classified.error };
        }
        recovery.markUncertain(candidate);
        return { kind: "uncertain" };
      }
    },
    [confirmForDirtyFiles, isFreshBranchActive, recovery, refreshStaleBranchPolicies],
  );
  return send;
}

async function resolveHandoffSession(
  recovery: TaskCreateRecovery,
  candidate: TaskCreationReceipt,
  isPassthroughProfile: boolean,
) {
  let sessionId = candidate.taskSessionId;
  const withAgent = candidate.payload.start_agent === true;
  const planMode = candidate.payload.plan_mode === true;
  if (!sessionId && (planMode || (withAgent && isPassthroughProfile))) {
    sessionId = await recovery.hydrateSessionId(candidate);
  }
  return { sessionId, withAgent, planMode };
}

type FinishHandoffArgs = {
  recovery: TaskCreateRecovery;
  candidate: TaskCreationReceipt;
  taskId: string;
  sessionId: string | null;
  planMode: boolean;
  withAgent: boolean;
  autoFocusNewTasks: boolean;
  isPassthroughProfile: boolean;
  clearDraft: () => void;
  onOpenChange: (open: boolean) => void;
  queueLastUsed: (payload: CreateTaskParams) => void;
  preserveLastUsedOnClose?: () => void;
  finishNavigation: (args: {
    planMode: boolean;
    newSessionId: string | null;
    autoFocusNewTasks: boolean;
    withAgent: boolean;
    isPassthroughProfile: boolean;
  }) => void;
};

function finishHandoff(args: FinishHandoffArgs) {
  args.queueLastUsed(args.candidate.payload);
  args.preserveLastUsedOnClose?.();
  args.clearDraft();
  args.onOpenChange(false);
  args.finishNavigation({
    planMode: args.planMode,
    newSessionId: args.sessionId,
    autoFocusNewTasks: args.autoFocusNewTasks,
    withAgent: args.withAgent,
    isPassthroughProfile: args.isPassthroughProfile,
  });
  args.recovery.acknowledge(args.candidate);
}

async function loadAcceptedTask(
  recovery: TaskCreateRecovery,
  candidate: TaskCreationReceipt,
  task: Task | CreateTaskResponse | null,
  isPassthroughProfile: boolean,
) {
  const session = await resolveHandoffSession(recovery, candidate, isPassthroughProfile);
  const response = task ?? (await recovery.hydrateTask(candidate));
  return { session, response };
}

export function useAcceptCreatedTask(args: {
  recovery: TaskCreateRecovery;
  matchesLiveDraft: (candidate: TaskCreationReceipt) => boolean;
  autoFocusNewTasks: boolean;
  isPassthroughProfile: boolean;
  onSuccess?: (
    task: Task,
    mode: "create" | "edit",
    meta?: { taskSessionId?: string | null; willNavigate?: boolean; autoFocus?: boolean },
  ) => void;
  onOpenChange: (open: boolean) => void;
  clearDraft: () => void;
  queueLastUsed: (payload: CreateTaskParams) => void;
  preserveLastUsedOnClose?: () => void;
  activatePlan: (taskId: string, sessionId: string) => void;
  openPassthroughTask: (taskId: string) => void;
}) {
  const {
    recovery,
    matchesLiveDraft,
    autoFocusNewTasks,
    isPassthroughProfile,
    onSuccess,
    onOpenChange,
    clearDraft,
    queueLastUsed,
    preserveLastUsedOnClose,
    activatePlan,
    openPassthroughTask,
  } = args;

  return useCallback(
    async (
      candidate: TaskCreationReceipt,
      task: Task | CreateTaskResponse | null,
      generation: number,
    ) => {
      if (candidate.state !== "accepted" || !recovery.isCurrent(candidate, generation)) {
        return;
      }
      if (candidate.handoffAttempted) return;
      if (!matchesLiveDraft(candidate)) return;

      const loaded = await loadAcceptedTask(recovery, candidate, task, isPassthroughProfile);
      if (!recovery.isCurrent(candidate, generation) || !matchesLiveDraft(candidate)) return;
      const taskId = candidate.taskId;
      if (!loaded.response || !taskId) return;
      const session = loaded.session;

      recovery.markHandoffAttempted(candidate);
      try {
        onSuccess?.(loaded.response, "create", {
          taskSessionId: session.sessionId,
          willNavigate:
            autoFocusNewTasks &&
            shouldNavigateAfterTaskCreate(
              session.withAgent,
              isPassthroughProfile,
              session.planMode,
              session.sessionId,
            ),
          autoFocus: autoFocusNewTasks,
        });
      } catch {
        return;
      }
      if (!recovery.isCurrent(candidate, generation) || !matchesLiveDraft(candidate)) return;

      finishHandoff({
        recovery,
        candidate,
        taskId,
        sessionId: session.sessionId,
        planMode: session.planMode,
        withAgent: session.withAgent,
        autoFocusNewTasks,
        isPassthroughProfile,
        clearDraft,
        onOpenChange,
        queueLastUsed,
        preserveLastUsedOnClose,
        finishNavigation: (nav) => {
          finishTaskCreateNavigation({
            ...nav,
            activatePlan: (sessionId) => activatePlan(taskId, sessionId),
            openPassthroughTask: () => openPassthroughTask(taskId),
          });
        },
      });
    },
    [args],
  );
}
