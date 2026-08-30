import type { Message } from "@/lib/types/http";

export type LatestMessageWindow = {
  messages: Message[];
  oldestCursor: string | null;
};

function compareMessages(a: Message, b: Message): number {
  const createdAtDifference = new Date(a.created_at).getTime() - new Date(b.created_at).getTime();
  if (createdAtDifference !== 0) return createdAtDifference;
  return a.id.localeCompare(b.id);
}

function joinMessages(base: Message[], extras: Message[]): Message[] {
  if (extras.length === 0) return base;
  const baseIds = new Set(base.map((message) => message.id));
  return [...base, ...extras.filter((message) => !baseIds.has(message.id))].sort(compareMessages);
}

/**
 * Reconcile a bounded newest-page response with the session cache while
 * preserving one contiguous pagination interval.
 */
export function reconcileLatestMessageWindow(params: {
  cachedAtRequest: Message[];
  cachedAtResponse: Message[];
  fetched: Message[];
}): LatestMessageWindow {
  const { cachedAtRequest, cachedAtResponse, fetched } = params;
  if (fetched.length === 0) {
    return {
      messages: cachedAtResponse,
      oldestCursor: cachedAtResponse[0]?.id ?? null,
    };
  }

  const fetchedIds = new Set(fetched.map((message) => message.id));
  const overlapsCachedWindow = cachedAtRequest.some((message) => fetchedIds.has(message.id));

  if (overlapsCachedWindow) {
    const messages = joinMessages(fetched, cachedAtResponse);
    return { messages, oldestCursor: messages[0]?.id ?? null };
  }

  const cachedAtRequestIds = new Set(cachedAtRequest.map((message) => message.id));
  const liveAdditions = cachedAtResponse.filter(
    (message) => !cachedAtRequestIds.has(message.id) && !fetchedIds.has(message.id),
  );
  const messages = joinMessages(fetched, liveAdditions);
  return { messages, oldestCursor: fetched[0]?.id ?? null };
}
