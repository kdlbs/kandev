"use client";

import { useCallback, useEffect, useRef, useState } from "react";
import type {
  PluginHostV2ManagedAgentInputReceipt,
  PluginManagedConversationController,
  PluginManagedConversationInputIntent,
  PluginManagedConversationClarificationAnswer,
  PluginManagedConversationSnapshot,
} from "@kandev/plugin-sdk";
import { issueHumanInteractionResponseReceipt } from "@/lib/plugins/human-interaction-receipts";
import { orderManagedConversationInputs } from "@/lib/plugins/managed-conversation";
import { generateUUID } from "@/lib/uuid";

// eslint-disable-next-line max-lines-per-function -- Conversation reads and actions share one immutable identity and stale-response guard.
export function useManagedConversationChat(
  initial: PluginManagedConversationSnapshot,
  controller: PluginManagedConversationController,
  onStatus?: (status: PluginManagedConversationSnapshot) => void,
) {
  const [status, setStatus] = useState(initial);
  const [inputs, setInputs] = useState<PluginHostV2ManagedAgentInputReceipt[]>([]);
  const [transportFailed, setTransportFailed] = useState(false);
  const [busy, setBusy] = useState(false);
  const [actionError, setActionError] = useState(false);
  const statusReadVersion = useRef(0);
  const inputReadVersion = useRef(0);
  const active = useRef(false);

  const acceptStatus = useCallback(
    (next: PluginManagedConversationSnapshot) => {
      if (!active.current) return;
      statusReadVersion.current += 1;
      setStatus(next);
      setTransportFailed(false);
      onStatus?.(next);
    },
    [onStatus],
  );

  const refreshStatus = useCallback(async () => {
    if (!active.current) return;
    const version = ++statusReadVersion.current;
    try {
      const next = await controller.getStatus();
      if (active.current && version === statusReadVersion.current) acceptStatus(next);
    } catch {
      if (active.current && version === statusReadVersion.current) setTransportFailed(true);
    }
  }, [acceptStatus, controller]);

  const refreshInputs = useCallback(async () => {
    if (!active.current) return;
    const version = ++inputReadVersion.current;
    try {
      const all: PluginHostV2ManagedAgentInputReceipt[] = [];
      let cursor = 0;
      for (let page = 0; page < 10; page += 1) {
        const result = await controller.listInputs({ sequenceCursor: cursor, limit: 100 });
        if (!active.current || version !== inputReadVersion.current) return;
        all.push(...result.inputs);
        if (!result.hasMore || result.nextSequenceCursor <= cursor) break;
        cursor = result.nextSequenceCursor;
      }
      if (active.current && version === inputReadVersion.current)
        setInputs(orderManagedConversationInputs(all));
    } catch {
      if (active.current && version === inputReadVersion.current) setTransportFailed(true);
    }
  }, [controller]);

  useEffect(() => {
    active.current = true;
    return () => {
      active.current = false;
      statusReadVersion.current += 1;
      inputReadVersion.current += 1;
    };
  }, []);

  useEffect(() => {
    acceptStatus(initial);
  }, [acceptStatus, initial]);

  useEffect(() => {
    void refreshStatus();
    void refreshInputs();
    const timer = window.setInterval(() => {
      void refreshStatus();
      void refreshInputs();
    }, 5_000);
    return () => window.clearInterval(timer);
  }, [refreshInputs, refreshStatus]);

  const execute = useCallback(
    async (action: () => Promise<void>) => {
      if (!active.current) return;
      setBusy(true);
      setActionError(false);
      try {
        await action();
        if (!active.current) return;
        await Promise.all([refreshStatus(), refreshInputs()]);
      } catch {
        if (active.current) setActionError(true);
      } finally {
        if (active.current) setBusy(false);
      }
    },
    [refreshInputs, refreshStatus],
  );

  const queueMessage = useCallback(
    async (intent: PluginManagedConversationInputIntent) => {
      if (!active.current) return;
      setBusy(true);
      setActionError(false);
      try {
        const result = await controller.enqueue({
          ...intent,
          expectedConversationRevision: status.revision,
        });
        if (active.current)
          setInputs((current) => orderManagedConversationInputs([...current, result.receipt]));
        await refreshStatus();
        await refreshInputs();
      } catch (error) {
        if (active.current) setActionError(true);
        throw error;
      } finally {
        if (active.current) setBusy(false);
      }
    },
    [controller, refreshInputs, refreshStatus, status.revision],
  );

  const cancelInput = useCallback(
    (receipt: PluginHostV2ManagedAgentInputReceipt) => {
      void execute(async () => {
        await controller.cancelInput({
          requestId: generateUUID(),
          idempotencyKey: generateUUID(),
          expectedConversationRevision: status.revision,
          hostInputId: receipt.hostInputId,
          expectedExecutionId: receipt.executionId || undefined,
        });
      });
    },
    [controller, execute, status.revision],
  );

  const togglePaused = useCallback(() => {
    void execute(async () => {
      acceptStatus(
        await controller.setPaused({
          requestId: generateUUID(),
          idempotencyKey: generateUUID(),
          expectedConversationRevision: status.revision,
          paused: status.state !== "paused",
        }),
      );
    });
  }, [acceptStatus, controller, execute, status.revision, status.state]);

  const recover = useCallback(() => {
    void execute(async () => {
      acceptStatus(
        await controller.recover({
          requestId: generateUUID(),
          idempotencyKey: generateUUID(),
          expectedConversationRevision: status.revision,
          expectedSessionResourceVersion: status.sessionResourceVersion,
          expectedExecutionId: status.executionId ?? "",
        }),
      );
    });
  }, [
    acceptStatus,
    controller,
    execute,
    status.executionId,
    status.revision,
    status.sessionResourceVersion,
  ]);

  const respondPermission = useCallback(
    (interactionId: string, expectedResourceVersion: string, optionId?: string) => {
      void execute(async () => {
        const receipt = await issueHumanInteractionResponseReceipt({
          workspaceId: status.workspaceId,
          interactionId,
          expectedResourceVersion,
          response: optionId
            ? { kind: "permission", optionId }
            : { kind: "permission", cancelled: true },
        });
        await controller.respondToPermission({
          requestId: generateUUID(),
          interactionId,
          expectedResourceVersion,
          optionId,
          cancelled: !optionId,
          humanResponseReceiptId: receipt.id,
        });
      });
    },
    [controller, execute, status.workspaceId],
  );

  const respondClarification = useCallback(
    (
      interactionId: string,
      expectedResourceVersion: string,
      answers: PluginManagedConversationClarificationAnswer[],
    ) => {
      void execute(async () => {
        const receipt = await issueHumanInteractionResponseReceipt({
          workspaceId: status.workspaceId,
          interactionId,
          expectedResourceVersion,
          response: {
            kind: "clarification",
            answers: answers.map((answer) => ({
              ...answer,
              selectedOptions: answer.selectedOptions?.length ? answer.selectedOptions : undefined,
              customText: answer.customText?.trim() || undefined,
            })),
          },
        });
        await controller.answerClarification({
          requestId: generateUUID(),
          interactionId,
          expectedResourceVersion,
          answers,
          humanResponseReceiptId: receipt.id,
        });
      });
    },
    [controller, execute, status.workspaceId],
  );

  return {
    status,
    inputs,
    transportFailed,
    busy,
    actionError,
    queueMessage,
    cancelInput,
    togglePaused,
    recover,
    respondPermission,
    respondClarification,
  };
}
