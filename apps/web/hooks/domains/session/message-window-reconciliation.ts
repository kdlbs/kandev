import type { Message } from "@/lib/types/http";

export type LatestMessageWindow = {
  messages: Message[];
  oldestCursor: string | null;
};

const MICROSECONDS_PER_MILLISECOND = BigInt(1_000);

/** Parse the same normalized-microsecond precision used by message pagination. */
function timestampMicros(createdAt: string): bigint | null {
  const match = createdAt.match(
    /^(\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2})(?:\.(\d+))?(Z|[+-]\d{2}:\d{2})$/,
  );
  if (!match) {
    const parsed = Date.parse(createdAt);
    return Number.isNaN(parsed) ? null : BigInt(parsed) * MICROSECONDS_PER_MILLISECOND;
  }

  const milliseconds = Date.parse(`${match[1]}${match[3]}`);
  if (Number.isNaN(milliseconds)) return null;
  const fraction = (match[2] ?? "").padEnd(6, "0").slice(0, 6);
  return BigInt(milliseconds) * MICROSECONDS_PER_MILLISECOND + BigInt(fraction);
}

function compareMessages(a: Message, b: Message): number {
  const aTimestamp = timestampMicros(a.created_at);
  const bTimestamp = timestampMicros(b.created_at);
  if (aTimestamp !== null && bTimestamp !== null) {
    if (aTimestamp !== bTimestamp) return aTimestamp < bTimestamp ? -1 : 1;
  } else if (a.created_at !== b.created_at) {
    return a.created_at.localeCompare(b.created_at);
  }
  if (a.id < b.id) return -1;
  if (a.id > b.id) return 1;
  return 0;
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

  const orderedFetched = [...fetched].sort(compareMessages);
  const fetchedBoundary = orderedFetched[0];
  const fetchedIds = new Set(orderedFetched.map((message) => message.id));
  const overlapsCachedWindow = cachedAtRequest.some((message) => fetchedIds.has(message.id));

  if (overlapsCachedWindow) {
    const messages = joinMessages(orderedFetched, cachedAtResponse);
    return { messages, oldestCursor: messages[0]?.id ?? null };
  }

  const cachedAtRequestIds = new Set(cachedAtRequest.map((message) => message.id));
  // A live row older than the fetched boundary belongs to the next page. If
  // retained here, the row would sit before `oldestCursor` and leave a gap in
  // the single contiguous interval represented by this window.
  const liveAdditions = cachedAtResponse.filter(
    (message) =>
      !cachedAtRequestIds.has(message.id) &&
      !fetchedIds.has(message.id) &&
      compareMessages(message, fetchedBoundary) >= 0,
  );
  const messages = joinMessages(orderedFetched, liveAdditions);
  return { messages, oldestCursor: fetchedBoundary.id };
}
