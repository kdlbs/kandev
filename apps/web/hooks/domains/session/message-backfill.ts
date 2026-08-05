import type { useAppStoreApi } from "@/components/state-provider";
import { listTaskSessionMessages } from "@/lib/api";
import { createDebugLogger } from "@/lib/debug/log";
import type { Message } from "@/lib/types/http";

const BACKFILL_PAGE_LIMIT = 100;
const debug = createDebugLogger("messages:fetch");

export const MAX_AUTO_BACKFILL_PAGES = 10;
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
): Promise<number> {
  const response = await listTaskSessionMessages(sessionId, {
    limit: BACKFILL_PAGE_LIMIT,
    before: oldestCursor,
    sort: "desc",
  });
  if (isInactive(isActive)) return 0;
  const ordered = [...(response.messages ?? [])].reverse();
  const newOldestCursor = ordered[0]?.id ?? oldestCursor;
  store.getState().prependMessages(sessionId, ordered, {
    hasMore: response.has_more ?? false,
    oldestCursor: newOldestCursor,
  });
  return ordered.length;
}

export async function runBackfillRound(
  sessionId: string,
  store: SessionMessageStore,
  round: number,
  isActive?: IsActive,
): Promise<BackfillStep> {
  if (isInactive(isActive)) return "stop";
  const meta = store.getState().messages.metaBySession[sessionId];
  const messages = store.getState().messages.bySession[sessionId] ?? [];
  if (hasUserOrAgentMessage(messages)) return "stop";
  if (!meta?.hasMore || !meta.oldestCursor) {
    debug("autoBackfill: stopping (no more older messages)", {
      sessionId,
      round,
      hasMore: meta?.hasMore ?? false,
    });
    return "stop";
  }
  debug("autoBackfill: window has no user/agent message, fetching older", {
    sessionId,
    round,
    currentCount: messages.length,
    oldestCursor: meta.oldestCursor,
  });
  try {
    const added = await fetchAndPrependOlder(sessionId, store, meta.oldestCursor, isActive);
    if (isInactive(isActive)) return "stop";
    return added === 0 ? "stop" : "continue";
  } catch (err) {
    debug("autoBackfill: fetch failed, stopping", { sessionId, round, err });
    return "stop";
  }
}

export async function autoBackfillUntilUserMessage(
  sessionId: string,
  store: SessionMessageStore,
  isActive?: IsActive,
): Promise<void> {
  for (let round = 0; round < MAX_AUTO_BACKFILL_PAGES; round++) {
    if (isInactive(isActive)) return;
    const step = await runBackfillRound(sessionId, store, round, isActive);
    if (step === "stop") return;
  }
  debug("autoBackfill: hit page budget without finding user/agent message", {
    sessionId,
    pageBudget: MAX_AUTO_BACKFILL_PAGES,
    messageBudget: MAX_AUTO_BACKFILL_PAGES * BACKFILL_PAGE_LIMIT,
  });
}
