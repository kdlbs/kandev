import type { StoreApi } from "zustand";
import type { AppState } from "@/lib/state/store";
import type { AvailableCommand } from "@/lib/state/slices/session-runtime/types";
import type { TurnEventPayload } from "@/lib/types/backend";
import type { Message } from "@/lib/types/http";
import { sessionId, taskId } from "@/lib/types/http";

/**
 * Extracts a leading `/command` token from a user prompt. Returns the bare
 * command name (no slash, original case) or null when the prompt does not start
 * with a slash command. Agent-advertised command names carry no leading slash,
 * so callers match the returned token case-insensitively against them.
 */
export function parseSlashCommand(content: string | undefined): string | null {
  const trimmed = content?.trim() ?? "";
  if (!trimmed.startsWith("/")) return null;
  const firstWord = trimmed.slice(1).split(/\s+/)[0] ?? "";
  return firstWord.length > 0 ? firstWord : null;
}

/** Whether `command` matches one of the agent's advertised commands (case-insensitive). */
export function isKnownCommand(
  command: string,
  available: AvailableCommand[] | undefined,
): boolean {
  const lower = command.toLowerCase();
  return (available ?? []).some((c) => c.name.toLowerCase() === lower);
}

const RESEND_HINT = "Try resending your request as a normal message, without the leading slash.";

/** The notice text shown for an empty turn, tailored to the triggering prompt. */
export function emptyTurnNoticeText(command: string | null, known: boolean): string {
  if (!command) {
    return "The agent finished without producing any output.";
  }
  if (!known) {
    return `\`/${command}\` isn't a command this agent recognizes, so it returned no output. ${RESEND_HINT}`;
  }
  return `\`/${command}\` ran but produced no output. ${RESEND_HINT}`;
}

export interface EmptyTurnNoticeInput {
  sessionId: string;
  taskId: string;
  turnId: string;
  hadOutput: boolean | undefined;
  isEphemeralSurface: boolean;
  messages: Message[] | undefined;
  availableCommands: AvailableCommand[] | undefined;
  now: string;
}

function noticeId(turnId: string): string {
  return `empty-turn-${turnId}`;
}

function findLastUserMessage(messages: Message[], turnId: string): Message | undefined {
  for (let i = messages.length - 1; i >= 0; i--) {
    const m = messages[i];
    if (m.author_type === "user" && m.turn_id === turnId) return m;
  }
  return undefined;
}

/**
 * Decides whether an empty-turn notice should be shown and, if so, returns the
 * synthetic status Message to inject. Pure: all store reads are passed in.
 *
 * Guards (all must pass): the turn explicitly had no output (orphan turns swept
 * on resume report had_output=true from the backend, so only genuine live
 * completions reach here); the surface is the main task chat (not quick-chat /
 * config-chat); and no notice exists yet for the turn (keyed by a deterministic
 * id, so it fires once).
 */
export function computeEmptyTurnNotice(input: EmptyTurnNoticeInput): Message | null {
  if (input.hadOutput !== false) return null;
  if (input.isEphemeralSurface) return null;

  const messages = input.messages ?? [];
  const id = noticeId(input.turnId);
  if (messages.some((m) => m.id === id)) return null;

  const userMessage = findLastUserMessage(messages, input.turnId);
  const command = parseSlashCommand(userMessage?.content);
  const known = command ? isKnownCommand(command, input.availableCommands) : false;
  const text = emptyTurnNoticeText(command, known);

  return {
    id,
    session_id: sessionId(input.sessionId),
    task_id: taskId(input.taskId),
    turn_id: input.turnId,
    author_type: "agent",
    content: text,
    type: "status",
    metadata: { variant: "warning", message: text, empty_turn: true },
    created_at: input.now,
  };
}

/**
 * Reads the store, computes the empty-turn notice for a completed turn, and
 * injects it into the chat when warranted.
 */
export function maybeEmitEmptyTurnNotice(
  store: StoreApi<AppState>,
  payload: TurnEventPayload,
  now: string = new Date().toISOString(),
): void {
  const state = store.getState();
  const sid = payload.session_id;
  const isEphemeralSurface =
    state.quickChat.sessions.some((s) => s.sessionId === sid) ||
    state.configChat.sessions.some((s) => s.sessionId === sid);

  const notice = computeEmptyTurnNotice({
    sessionId: sid,
    taskId: payload.task_id,
    turnId: payload.id,
    hadOutput: payload.had_output,
    isEphemeralSurface,
    messages: state.messages.bySession[sid],
    availableCommands: state.availableCommands.bySessionId[sid],
    now,
  });

  if (notice) store.getState().addMessage(notice);
}
