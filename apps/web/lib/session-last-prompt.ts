import type { Message } from "@/lib/types/http";
import { TASK_DESCRIPTION_SYNTHETIC_ID } from "@/hooks/initial-prompt-preview";
import {
  compareMessageTimestamps,
  messageTimestampNanoseconds,
} from "@/lib/state/slices/session/message-timestamp";

export type ObservedPrompts = {
  ids: Record<string, true>;
  newestKey: Pick<Message, "created_at" | "id"> | null;
};

export function isStoredUserPrompt(message: Message): boolean {
  return message.author_type === "user" && message.id !== TASK_DESCRIPTION_SYNTHETIC_ID;
}

export function isValidPromptMessage(message: Message): boolean {
  return isStoredUserPrompt(message) && messageTimestampNanoseconds(message.created_at) !== null;
}

function compareIds(left: string, right: string): number {
  if (left < right) return -1;
  if (left > right) return 1;
  return 0;
}

export function comparePromptOrder(
  left: Pick<Message, "created_at" | "id">,
  right: Pick<Message, "created_at" | "id">,
): number | null {
  const time = compareMessageTimestamps(left.created_at, right.created_at);
  if (time === null) return null;
  return time === 0 ? compareIds(left.id, right.id) : time;
}

export function findLastStoredUserPromptIndex(messages: readonly Message[]): number {
  for (let index = messages.length - 1; index >= 0; index--) {
    if (isStoredUserPrompt(messages[index])) return index;
  }
  return -1;
}

function compareCandidates(left: Message, right: Message): number {
  const comparison = comparePromptOrder(left, right);
  if (comparison !== null) return comparison;
  const leftValid = isValidPromptMessage(left);
  const rightValid = isValidPromptMessage(right);
  if (leftValid !== rightValid) return leftValid ? 1 : -1;
  return compareIds(left.id, right.id);
}

function newestAdmittedProjection(
  projection: readonly Message[],
  observed: ObservedPrompts | undefined,
): Message | null {
  if (!observed) return null;
  let newest: Message | null = null;
  for (const message of projection) {
    if (!isValidPromptMessage(message)) continue;
    const order = observed.newestKey ? comparePromptOrder(message, observed.newestKey) : null;
    if (observed.ids[message.id] !== true && (order === null || order <= 0)) continue;
    if (!newest || compareCandidates(message, newest) > 0) newest = message;
  }
  return newest;
}

export function resolveLastPromptMessage(
  windowMessages: readonly Message[],
  promptProjection: readonly Message[],
  observed: ObservedPrompts | undefined,
): Message | null {
  const windowIndex = findLastStoredUserPromptIndex(windowMessages);
  const windowLast = windowIndex < 0 ? null : windowMessages[windowIndex];
  const projectionLast = newestAdmittedProjection(promptProjection, observed);
  if (!windowLast) return projectionLast;
  if (!projectionLast) return windowLast;
  const order = compareCandidates(projectionLast, windowLast);
  if (order !== 0) return order > 0 ? projectionLast : windowLast;
  const updated = compareMessageTimestamps(projectionLast.updated_at, windowLast.updated_at);
  return updated !== null && updated > 0 ? projectionLast : windowLast;
}
