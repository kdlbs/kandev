import type { Message } from "@/lib/types/http";
import { parseTurnTimestamp } from "./turn-actions";

const ZERO = BigInt(0);
const THOUSAND = BigInt(1000);
const NINE_HUNDRED_NINETY_NINE = BigInt(999);

/** Returns a strict RFC3339 timestamp in epoch nanoseconds. */
export function messageTimestampNanoseconds(value: string | undefined): bigint | null {
  return parseTurnTimestamp(value);
}

/** Returns the backend ordering key, which truncates an instant to microseconds. */
export function messageTimestampMicros(value: string | undefined): bigint | null {
  const timestamp = messageTimestampNanoseconds(value);
  if (timestamp === null) return null;
  return timestamp >= ZERO
    ? timestamp / THOUSAND
    : -((-timestamp + NINE_HUNDRED_NINETY_NINE) / THOUSAND);
}

/** Compares two message timestamps with the precision used by the backend. */
export function compareMessageTimestamps(left: string | undefined, right: string | undefined) {
  const leftMicros = messageTimestampMicros(left);
  const rightMicros = messageTimestampMicros(right);
  if (leftMicros === null || rightMicros === null) return null;
  if (leftMicros < rightMicros) return -1;
  if (leftMicros > rightMicros) return 1;
  return 0;
}

/** Reports whether an incoming row is at least as fresh as a cached row. */
export function isIncomingMessageAtLeastAsFresh(existing: Message, incoming: Message) {
  const existingTimestamp = messageTimestampNanoseconds(existing.updated_at);
  const incomingTimestamp = messageTimestampNanoseconds(incoming.updated_at);
  if (existingTimestamp === null) return true;
  if (incomingTimestamp === null) return false;
  return incomingTimestamp >= existingTimestamp;
}
