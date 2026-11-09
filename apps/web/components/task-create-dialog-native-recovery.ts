"use client";

import { useCallback } from "react";
import { t } from "@/lib/i18n";
import { linkToTask } from "@/lib/links";
import type { createTask } from "@/lib/api";
import type { CreateTaskResponse } from "@/lib/types/http";
import { decodeNativeCreateResponse, type TaskCreationReceipt } from "@/lib/task-creation-receipt";
import type {
  TaskCreateRecovery,
  RecoveryLookupResult,
} from "@/components/task-create-dialog-recovery";
import type {
  NativeSendResult,
  useAcceptCreatedTask,
} from "@/components/task-create-dialog-native-handoff";

type AcceptCreatedTask = ReturnType<typeof useAcceptCreatedTask>;

async function presentAcceptedLookup(args: {
  recovery: TaskCreateRecovery;
  acceptCreatedTask: AcceptCreatedTask;
  next: TaskCreationReceipt;
  generation: number;
}) {
  if (!args.recovery.isCurrent(args.next, args.generation)) return;
  const task = await args.recovery.hydrateTask(args.next);
  if (!args.recovery.isCurrent(args.next, args.generation)) return;
  await args.acceptCreatedTask(args.next, task, args.generation);
}

async function executeRetry(args: {
  recovery: TaskCreateRecovery;
  generation: number;
  sendNativeCreate: (
    candidate: TaskCreationReceipt,
    reconsented: boolean,
    generation: number,
  ) => Promise<NativeSendResult>;
  applyObservation: (
    candidate: TaskCreationReceipt,
    observation: ReturnType<typeof decodeNativeCreateResponse>,
    response: CreateTaskResponse | undefined,
    generation: number,
  ) => Promise<void>;
  acceptCreatedTask: AcceptCreatedTask;
  errorMessage: (error: unknown) => string;
  toast: (input: { title: string; description: string; variant?: "error" }) => unknown;
}) {
  const { recovery, generation } = args;
  const current = recovery.getReceipt();
  if (!current) return;
  const observation: RecoveryLookupResult = await recovery.lookup(current);
  if (observation.kind !== "absent") {
    const next = recovery.applyLookupResult(current, observation);
    if (next?.state === "accepted") {
      await presentAcceptedLookup({
        recovery,
        acceptCreatedTask: args.acceptCreatedTask,
        next,
        generation,
      });
    }
    return;
  }
  const next = recovery.applyLookupMiss(current);
  if (!next || !recovery.isOwnerCurrent(generation)) return;
  if (!next.replayable || next.taskId) return;
  const result = await args.sendNativeCreate(next, false, generation);
  if (result.kind === "response") {
    await args.applyObservation(
      next,
      decodeNativeCreateResponse(result.response, next),
      result.response,
      generation,
    );
  } else if (result.kind === "rejected" && recovery.isOwnerCurrent(generation)) {
    args.toast({
      title: t("task:failedToCreateTask"),
      description: args.errorMessage(result.error),
      variant: "error",
    });
  }
}

export function useNativeSubmission(args: {
  recovery: TaskCreateRecovery;
  acceptCreatedTask: AcceptCreatedTask;
  sendNativeCreate: (
    candidate: TaskCreationReceipt,
    reconsented: boolean,
    generation: number,
  ) => Promise<NativeSendResult>;
  getDraftTitle: () => string;
  errorMessage: (error: unknown) => string;
  toast: (input: { title: string; description: string; variant?: "error" }) => unknown;
}) {
  const { recovery, acceptCreatedTask, sendNativeCreate, getDraftTitle, errorMessage, toast } =
    args;

  const applyNativeObservation = useCallback(
    async (
      candidate: TaskCreationReceipt,
      observation: ReturnType<typeof decodeNativeCreateResponse>,
      response: CreateTaskResponse | undefined,
      generation: number,
    ) => {
      const next = recovery.applyCreateResult(candidate, observation);
      if (!next || !recovery.isCurrent(next, generation)) return;
      if (next.state !== "accepted") return;
      if (response?.queued_for_step_id) {
        toast({
          title: t("task:taskQueued"),
          description: t("task:taskQueuedWipLimit"),
        });
      }
      await acceptCreatedTask(next, response ?? null, generation);
    },
    [acceptCreatedTask, recovery, toast],
  );

  const runNativeSubmission = useCallback(
    async (
      buildPayload: (consented: string[]) => Parameters<typeof createTask>[0],
      consent: string[],
      generation: number,
    ) => {
      if (!recovery.isOwnerCurrent(generation)) return;
      const payload = buildPayload(consent);
      const began = recovery.beginSubmission(payload, getDraftTitle());
      if (!began) return;
      if (began.retrying) {
        await executeRetry({
          recovery,
          generation: began.generation,
          sendNativeCreate,
          applyObservation: applyNativeObservation,
          acceptCreatedTask,
          errorMessage,
          toast,
        });
        return;
      }
      const result = await sendNativeCreate(began.receipt, false, began.generation);
      if (result.kind === "rejected") {
        if (recovery.isOwnerCurrent(began.generation)) throw result.error;
        return;
      }
      if (result.kind !== "response") return;
      await applyNativeObservation(
        began.receipt,
        decodeNativeCreateResponse(result.response, began.receipt),
        result.response,
        began.generation,
      );
    },
    [
      acceptCreatedTask,
      applyNativeObservation,
      errorMessage,
      getDraftTitle,
      recovery,
      sendNativeCreate,
      toast,
    ],
  );

  return { applyNativeObservation, runNativeSubmission };
}

export function useRecoveryLookupActions(args: {
  recovery: TaskCreateRecovery;
  acceptCreatedTask: AcceptCreatedTask;
  sendNativeCreate: (
    candidate: TaskCreationReceipt,
    reconsented: boolean,
    generation: number,
  ) => Promise<NativeSendResult>;
  applyObservation: (
    candidate: TaskCreationReceipt,
    observation: ReturnType<typeof decodeNativeCreateResponse>,
    response: CreateTaskResponse | undefined,
    generation: number,
  ) => Promise<void>;
  errorMessage: (error: unknown) => string;
  toast: (input: { title: string; description: string; variant?: "error" }) => unknown;
}) {
  const { recovery, acceptCreatedTask, sendNativeCreate, applyObservation, errorMessage, toast } =
    args;

  const checkStatus = useCallback(async () => {
    const generation = recovery.getGeneration();
    const current = recovery.getReceipt();
    if (!current || !recovery.beginOperation()) return;
    try {
      const observation = await recovery.lookup(current);
      const next = recovery.applyLookupResult(current, observation);
      if (next?.state === "accepted") {
        await presentAcceptedLookup({ recovery, acceptCreatedTask, next, generation });
      }
    } finally {
      recovery.endOperation();
    }
  }, [acceptCreatedTask, recovery]);

  const retry = useCallback(async () => {
    const generation = recovery.getGeneration();
    if (!recovery.beginOperation()) return;
    try {
      await executeRetry({
        recovery,
        generation,
        sendNativeCreate,
        applyObservation,
        acceptCreatedTask,
        errorMessage,
        toast,
      });
    } finally {
      recovery.endOperation();
    }
  }, [acceptCreatedTask, applyObservation, errorMessage, recovery, sendNativeCreate, toast]);

  return { checkStatus, retry };
}

export function useRecoveryTaskOpen(args: {
  recovery: TaskCreateRecovery;
  onOpenChange: (open: boolean) => void;
  router: { push: (href: string) => void };
}) {
  const { recovery, onOpenChange, router } = args;
  return useCallback(async () => {
    const generation = recovery.getGeneration();
    const current = recovery.getReceipt();
    if (!current?.taskId || current.state === "unavailable" || !recovery.beginOperation()) {
      return;
    }
    try {
      const task = await recovery.hydrateTask(current);
      if (!task || !recovery.isOwnerCurrent(generation)) return;
      const latest = recovery.getReceipt();
      if (!latest?.taskId || latest.state === "unavailable") return;
      onOpenChange(false);
      const href =
        latest.state !== "accepted"
          ? linkToTask(latest.taskId, {
              searchParams: new URLSearchParams({ creation: "pending" }),
            })
          : linkToTask(latest.taskId);
      router.push(href);
      recovery.acknowledge(latest);
    } finally {
      recovery.endOperation();
    }
  }, [onOpenChange, recovery, router]);
}
