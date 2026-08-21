import type { useAppStoreApi } from "@/components/state-provider";
import { createDebugLogger } from "@/lib/debug/log";
import type { Message } from "@/lib/types/http";
import { requestOlderMessages } from "./older-message-pagination";

const BACKFILL_PAGE_LIMIT = 100;
const debug = createDebugLogger("messages:fetch");

export const MAX_AUTO_BACKFILL_PAGES = 10;
// The backfill ceiling is a MESSAGE budget (was MAX_AUTO_BACKFILL_PAGES *
// BACKFILL_PAGE_LIMIT pages); the permitted page count derives from the
// coordinator's actual first-request-wins page size, so a 20-row winner
// reaches the same maximum message depth as a 100-row winner.
export const MAX_AUTO_BACKFILL_MESSAGES = MAX_AUTO_BACKFILL_PAGES * BACKFILL_PAGE_LIMIT;
export type BackfillStep = "continue" | "stop";

type IsActive = () => boolean;
type SessionMessageStore = ReturnType<typeof useAppStoreApi>;

function isInactive(isActive?: IsActive): boolean {
  return isActive !== undefined && !isActive();
}

export function hasUserOrAgentMessage(messages: Message[]): boolean {
  return messages.some(
    (message) =>
      message.type === "message" &&
      (message.author_type === "user" || message.author_type === "agent"),
  );
}

async function fetchAndPrependOlder(
  sessionId: string,
  store: SessionMessageStore,
  oldestCursor: string,
  isActive?: IsActive,
): Promise<{ count: number; effectiveLimit: number }> {
  const result = await requestOlderMessages({
    sessionId,
    cursor: oldestCursor,
    limit: BACKFILL_PAGE_LIMIT,
    store,
  });
  if (isInactive(isActive)) return { count: 0, effectiveLimit: result.effectiveLimit };
  return { count: result.count, effectiveLimit: result.effectiveLimit };
}

async function runBackfillRoundWithBudget(
  sessionId: string,
  store: SessionMessageStore,
  round: number,
  isActive?: IsActive,
): Promise<{ step: BackfillStep; consumed: number }> {
  if (isInactive(isActive)) return { step: "stop", consumed: 0 };
  const meta = store.getState().messages.metaBySession[sessionId];
  const messages = store.getState().messages.bySession[sessionId] ?? [];
  if (hasUserOrAgentMessage(messages)) return { step: "stop", consumed: 0 };
  if (!meta?.hasMore || !meta.oldestCursor) {
    debug("autoBackfill: stopping (no more older messages)", {
      sessionId,
      round,
      hasMore: meta?.hasMore ?? false,
    });
    return { step: "stop", consumed: 0 };
  }
  debug("autoBackfill: window has no user/agent message, fetching older", {
    sessionId,
    round,
    currentCount: messages.length,
    oldestCursor: meta.oldestCursor,
  });
  try {
    const { count, effectiveLimit } = await fetchAndPrependOlder(
      sessionId,
      store,
      meta.oldestCursor,
      isActive,
    );
    if (isInactive(isActive)) return { step: "stop", consumed: 0 };
    return { step: count === 0 ? "stop" : "continue", consumed: effectiveLimit };
  } catch (err) {
    debug("autoBackfill: fetch failed, stopping", { sessionId, round, err });
    return { step: "stop", consumed: 0 };
  }
}

export async function runBackfillRound(
  sessionId: string,
  store: SessionMessageStore,
  round: number,
  isActive?: IsActive,
): Promise<BackfillStep> {
  const { step } = await runBackfillRoundWithBudget(sessionId, store, round, isActive);
  return step;
}

export async function autoBackfillUntilUserMessage(
  sessionId: string,
  store: SessionMessageStore,
  isActive?: IsActive,
): Promise<void> {
  let messageBudget = MAX_AUTO_BACKFILL_MESSAGES;
  let round = 0;
  while (messageBudget > 0) {
    if (isInactive(isActive)) return;
    const { step, consumed } = await runBackfillRoundWithBudget(sessionId, store, round, isActive);
    if (step === "stop") return;
    messageBudget -= consumed;
    round += 1;
  }
  debug("autoBackfill: hit message budget without finding user/agent message", {
    sessionId,
    pageBudget: round,
    messageBudget: MAX_AUTO_BACKFILL_MESSAGES,
  });
}
