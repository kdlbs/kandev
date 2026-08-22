import type { Message } from "@/lib/types/http";

const SEP = "\u0000";
const signatureCache = new WeakMap<Message, string>();

/**
 * djb2 string hash → unsigned base36. Non-crypto; paired with a length prefix
 * and the message id (during reconciliation) so collisions cannot cause a
 * visible reuse of a changed message.
 */
function hashString(input: string): string {
  let hash = 5381;
  for (let i = 0; i < input.length; i++) {
    hash = ((hash << 5) + hash) ^ input.charCodeAt(i);
  }
  return (hash >>> 0).toString(36);
}

function serializeMetadata(metadata: Record<string, unknown> | undefined): string {
  if (!metadata) return "";
  // Sort keys so signature is stable regardless of insertion order.
  const keys = Object.keys(metadata).sort();
  let out = "";
  for (const key of keys) out += `${key}=${JSON.stringify(metadata[key])}${SEP}`;
  return out;
}

function contentHashSignature(message: Message): string {
  const parts =
    message.content +
    SEP +
    message.type +
    SEP +
    (message.turn_id ?? "") +
    SEP +
    (message.requests_input ? "1" : "0") +
    SEP +
    (message.raw_content ?? "") +
    SEP +
    serializeMetadata(message.metadata) +
    SEP +
    (message.prompt_index ?? "");
  return `h:${parts.length}:${hashString(parts)}`;
}

/**
 * Content signature of a message, cached per-object via a WeakMap so each
 * Message object is hashed at most once. Two messages with the same id and the
 * same signature are treated as unchanged for reconciliation. The WeakMap is
 * keyed by the Message object, so entries are GC'd once messages leave the
 * store (e.g. task delete/archive) — no cap or cleanup hook needed.
 *
 * When the backend supplies `updated_at` (the authoritative per-message change
 * signal, bumped on every content/metadata mutation including streaming tokens),
 * the signature short-circuits to it — an O(1) compare with no content hashing.
 * `prompt_index` is included in BOTH branches: a refetch that newly supplies an
 * ordinal (or replaces it) is a meaningful snapshot difference even when
 * `updated_at` is unchanged. Older payloads without `updated_at` fall back to
 * the content hash.
 */
export function signatureOf(message: Message): string {
  const cached = signatureCache.get(message);
  if (cached !== undefined) return cached;
  const signature = message.updated_at
    ? `u:${message.updated_at}:p${message.prompt_index ?? ""}`
    : contentHashSignature(message);
  signatureCache.set(message, signature);
  return signature;
}

/**
 * Reconcile a full snapshot against the previous array, preserving object
 * identity for messages whose signature is unchanged and returning the prev
 * array reference itself when nothing moved (so array identity is preserved and
 * no downstream re-render fires). Only genuinely-changed messages get a fresh
 * reference.
 *
 * Before comparing, a merged snapshot carries forward a previously known
 * `prompt_index` onto incoming payloads that omit the field, so an older or
 * transient payload can never clear a known ordinal; a later HTTP/boot
 * snapshot that supplies (or replaces) the index still wins through the
 * signature.
 */
export function reconcileMessages(prev: Message[] | undefined, next: Message[]): Message[] {
  if (!prev || prev.length === 0) return next;
  const prevById = new Map<string, Message>();
  for (const message of prev) prevById.set(message.id, message);
  // Carry forward a known prompt_index when the incoming payload omits it.
  const carried = next.map((message) => {
    const previous = prevById.get(message.id);
    if (
      previous &&
      message.prompt_index === undefined &&
      typeof previous.prompt_index === "number"
    ) {
      return { ...message, prompt_index: previous.prompt_index };
    }
    return message;
  });
  let identical = prev.length === carried.length;
  const result = carried.map((message, i) => {
    const previous = prevById.get(message.id);
    const reused = previous && signatureOf(previous) === signatureOf(message) ? previous : message;
    if (reused !== prev[i]) identical = false;
    return reused;
  });
  return identical ? prev : result;
}
