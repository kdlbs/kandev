import type { StoreApi } from "zustand";
import type { AppState } from "@/lib/state/store";
import type { AvailableCommand } from "@/lib/state/slices/session-runtime/types";
import type { ConfigurationCommandResult, TurnEventPayload } from "@/lib/types/backend";
import type { Message } from "@/lib/types/http";
import { sessionId, taskId } from "@/lib/types/http";
import { isPassthroughSession } from "@/lib/session/is-passthrough-session";
import { t } from "@/lib/i18n";

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

// Catalog key; resolved inside `emptyTurnNoticeText`.
const RESEND_HINT_KEY = "chat:emptyTurnResendHint";

/** The notice text shown for an empty turn, tailored to the triggering prompt. */
export function emptyTurnNoticeText(command: string | null, known: boolean): string {
  if (!command) {
    return t("chat:emptyTurnNoOutput");
  }
  const hint = t(RESEND_HINT_KEY);
  if (!known) {
    return t("chat:emptyTurnUnknownCommand", { command, hint });
  }
  return t("chat:emptyTurnKnownCommand", { command, hint });
}

export interface EmptyTurnNoticeInput {
  sessionId: string;
  taskId: string;
  turnId: string;
  hadOutput: boolean | undefined;
  metadata?: Record<string, unknown>;
  isEphemeralSurface: boolean;
  isPassthrough?: boolean;
  messages: Message[] | undefined;
  availableCommands: AvailableCommand[] | undefined;
  configuration_result?: ConfigurationCommandResult;
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

// hasSubagentTaskMessage returns true when the turn contains a Task (subagent)
// tool call message. Guards against the empty-turn race when a parent agent
// dispatches a subagent: the Claude Agent SDK may return session/prompt with
// stop_reason while the subagent still runs (anthropics/claude-code#47936), so
// the backend's had_output snapshot can be false even though the turn dispatched
// real work and the subagent's nested events will land seconds later.
function hasSubagentTaskMessage(messages: Message[], turnId: string): boolean {
  for (const m of messages) {
    if (m.turn_id !== turnId || m.author_type !== "agent") continue;
    const meta = m.metadata as { normalized?: { kind?: string } } | undefined;
    if (meta?.normalized?.kind === "subagent_task") return true;
  }
  return false;
}

function shouldEmitEmptyTurnNotice(
  input: EmptyTurnNoticeInput,
  messages: Message[],
  id: string,
): boolean {
  return (
    input.hadOutput === false &&
    input.isPassthrough !== true &&
    input.metadata?.lifecycle_only !== true &&
    input.metadata?.error_terminated !== true &&
    !input.isEphemeralSurface &&
    !messages.some((message) => message.id === id) &&
    !hasSubagentTaskMessage(messages, input.turnId)
  );
}

function configurationCommandNoticeText(result: ConfigurationCommandResult): string {
  const requested = result.requested.trim();
  const effective = result.effective?.trim() ?? "";
  const status = configurationCommandFeedbackStatus(result);
  if (status === "applied") {
    return t("chat:configurationCommandApplied", { value: effective });
  }
  if (status === "clamped") {
    return t("chat:configurationCommandClamped", { requested, effective });
  }
  if (effective) {
    return t("chat:configurationCommandUnconfirmedWithObservation", { requested, effective });
  }
  return t("chat:configurationCommandUnconfirmed", { requested });
}

function configurationCommandFeedbackStatus(
  result: ConfigurationCommandResult,
): ConfigurationCommandResult["status"] {
  if (result.status === "clamped" && result.effective?.trim()) return "clamped";
  if (!result.confirmed || !result.effective?.trim()) return "unconfirmed";
  return result.status === "applied" || result.status === "clamped" ? result.status : "unconfirmed";
}

/**
 * Decides whether an empty-turn notice should be shown and, if so, returns the
 * synthetic status Message to inject. Pure: all store reads are passed in.
 *
 * Guards (all must pass): the turn explicitly had no output; it is a user turn
 * rather than synthetic lifecycle history or a failure outcome; the surface is
 * the main task chat (not quick-chat/config-chat); and no notice exists yet for
 * the turn (keyed by a deterministic id, so it fires once).
 */
export function computeEmptyTurnNotice(input: EmptyTurnNoticeInput): Message | null {
  const messages = input.messages ?? [];
  const id = noticeId(input.turnId);
  if (!shouldEmitEmptyTurnNotice(input, messages, id)) return null;

  if (input.configuration_result) {
    const result = input.configuration_result;
    if (!result.config_id?.trim()) return null;
    const status = configurationCommandFeedbackStatus(result);
    const text = configurationCommandNoticeText(result);
    return {
      id,
      session_id: sessionId(input.sessionId),
      task_id: taskId(input.taskId),
      turn_id: input.turnId,
      author_type: "agent",
      content: text,
      type: "status",
      metadata: {
        variant: "configuration_command_feedback",
        message: text,
        empty_turn: true,
        configuration_command_feedback: true,
        feedback_status: status,
        dismissible: status === "unconfirmed",
      },
      created_at: input.now,
    };
  }

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
  const id = noticeId(payload.id);
  const messages = state.messages.bySession[sid] ?? [];
  const existing = messages.find((message) => message.id === id);
  const isEphemeralSurface = state.quickChat.sessions.some((s) => s.sessionId === sid);

  const notice = computeEmptyTurnNotice({
    sessionId: sid,
    taskId: payload.task_id,
    turnId: payload.id,
    hadOutput: payload.had_output,
    metadata: payload.metadata,
    isEphemeralSurface,
    isPassthrough: isPassthroughSession(state.taskSessions.items[sid]),
    messages: existing ? messages.filter((message) => message.id !== id) : messages,
    availableCommands: state.availableCommands.bySessionId[sid],
    configuration_result: payload.configuration_result,
    now,
  });

  if (!notice) return;
  if (existing) {
    if (
      payload.configuration_result &&
      notice.metadata?.configuration_command_feedback === true &&
      existing.metadata?.configuration_command_feedback !== true
    ) {
      store.getState().updateMessage({
        ...existing,
        content: notice.content,
        metadata: notice.metadata,
      });
    }
    return;
  }
  if (state.claimEmptyTurnNotice(sid, payload.id)) state.addMessage(notice);
}
