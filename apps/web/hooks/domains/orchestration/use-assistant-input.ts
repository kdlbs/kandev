import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { ApiError } from "@/lib/api/client";
import { generateUUID } from "@/lib/utils";
import {
  getAssistantInput,
  resolveAssistantInput,
  type AssistantAnswer,
  type AssistantAttention,
  type AssistantBinding,
  type AssistantInputSnapshot,
} from "@/lib/api/domains/assistant-api";
import type { ClarificationTransport } from "@/components/task/chat/clarification-transport";
export function useAssistantInput(
  binding: AssistantBinding,
  row: AssistantAttention,
  onResolved: () => void,
) {
  const [snapshot, setSnapshot] = useState<AssistantInputSnapshot>();
  const [error, setError] = useState<unknown>();
  const [busy, setBusy] = useState(false);
  const [revision, setRevision] = useState(0);
  const generation = useRef(0);
  const inflight = useRef(false);
  const attempt = useRef<{ identity: string; body: AssistantAnswer } | undefined>(undefined);
  const refresh = useCallback(() => setRevision((value) => value + 1), []);
  useEffect(() => {
    const current = ++generation.current;
    const controller = new AbortController();
    setSnapshot(undefined);
    setError(undefined);
    setBusy(false);
    void getAssistantInput(row.id, controller.signal)
      .then((value) => {
        if (current === generation.current) setSnapshot(value);
      })
      .catch((cause) => {
        if (current === generation.current) setError(cause);
      });
    return () => {
      ++generation.current;
      controller.abort();
    };
  }, [binding.id, binding.version, row.id, row.revision, revision]);
  const resolve = useCallback(
    async (
      payload: Pick<AssistantAnswer, "answers" | "rejected" | "reject_reason" | "option_id">,
    ) => {
      if (!snapshot || inflight.current) return false;
      inflight.current = true;
      setBusy(true);
      setError(undefined);
      const current = generation.current;
      const identity = JSON.stringify({ source: snapshot.attention.source_revision, ...payload });
      if (attempt.current?.identity !== identity) {
        attempt.current = {
          identity,
          body: {
            ...payload,
            operation_id: generateUUID(),
            expected_intent_revision: binding.intent_revision,
            expected_binding_version: binding.version,
            expected_revision: snapshot.attention.revision,
            source_revision: snapshot.attention.source_revision,
            session_id: snapshot.input.session_id,
          },
        };
      }
      try {
        await resolveAssistantInput(row.id, attempt.current.body);
        if (current === generation.current) {
          attempt.current = undefined;
          onResolved();
          refresh();
        }
        return true;
      } catch (cause) {
        if (current === generation.current) {
          setError(cause);
          if (
            cause instanceof ApiError &&
            cause.status === 409 &&
            cause.message !== "operation_outcome_unknown"
          ) {
            attempt.current = undefined;
            onResolved();
            refresh();
          }
        }
        return false;
      } finally {
        inflight.current = false;
        if (current === generation.current) setBusy(false);
      }
    },
    [snapshot, binding.intent_revision, binding.version, row.id, onResolved, refresh],
  );
  const transport = useInputTransport(resolve, snapshot?.input.pending_id);
  return { snapshot, error, busy, refresh, resolve, transport };
}

function useInputTransport(
  resolve: (
    payload: Pick<AssistantAnswer, "answers" | "rejected" | "reject_reason" | "option_id">,
  ) => Promise<boolean>,
  pendingId?: string,
) {
  return useMemo<ClarificationTransport>(
    () => ({
      respond: async (handle, body) => {
        if (handle !== pendingId) return { state: "expired" };
        const ok = await resolve(body);
        return ok
          ? {
              state: "ok",
              claimed: true,
              status: body.rejected ? "rejected" : "answered",
              answers: body.answers,
            }
          : { state: "error" };
      },
      updateMessage: () => {},
    }),
    [resolve, pendingId],
  );
}
