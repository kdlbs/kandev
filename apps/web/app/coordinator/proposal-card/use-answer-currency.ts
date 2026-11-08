"use client";

import { useEffect, useState } from "react";
import type { AnswerSpec } from "@/lib/api/domains/coordinator-api";
import { readRelay, type Relay } from "@/lib/api/domains/coordinator-relay-api";

export type AnswerCurrency = "unknown" | "current" | "stale";

function requestId(relay: Relay): string {
  const metadata = relay.permission?.message.metadata as
    | { request_id?: string; pending_id?: string }
    | undefined;
  return `${metadata?.request_id ?? ""}|${metadata?.pending_id ?? ""}`;
}

/** Whether the live relay still shows the target this answer was drafted for. */
export function answerCurrencyOf(spec: AnswerSpec, relay: Relay): AnswerCurrency {
  if (spec.target === "clarification") {
    return relay.clarification?.pending_id === spec.pending_id ? "current" : "stale";
  }
  if (!relay.permission) return "stale";
  return requestId(relay) === `${spec.request_id ?? ""}|${spec.pending_id}` ? "current" : "stale";
}

/**
 * Reads the live relay once when `enabled` turns on, to tell the Edit view
 * whether the question is still the one this answer was drafted for. A read
 * that fails or is refused leaves the answer "stale": the stored draft is used.
 */
export function useAnswerCurrency(
  workspaceId: string,
  coordinatorId: string,
  spec: AnswerSpec,
  enabled: boolean,
): AnswerCurrency {
  const [result, setResult] = useState<AnswerCurrency>("unknown");
  useEffect(() => {
    if (!enabled) return;
    let cancelled = false;
    void readRelay(workspaceId, coordinatorId, spec.task_id).then((read) => {
      if (cancelled) return;
      setResult(read.kind === "ok" ? answerCurrencyOf(spec, read.relay) : "stale");
    });
    return () => {
      cancelled = true;
    };
    // The spec identity fields are what the read compares against.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [enabled, workspaceId, coordinatorId, spec.task_id, spec.pending_id, spec.request_id]);
  return enabled ? result : "unknown";
}
