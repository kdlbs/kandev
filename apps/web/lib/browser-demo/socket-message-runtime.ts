import { demoQueueStatus, handleFollowUp } from "./agent-queue-runtime";
import { respond, state } from "./runtime-state";

export function routeMessageList(
  socketId: string,
  id: string,
  action: string,
  payload: Record<string, unknown>,
): boolean {
  if (action === "message.list") {
    respond(socketId, id, {
      messages: [...(state.messagesBySession[String(payload.session_id)] ?? [])].reverse(),
      has_more: false,
    });
    return true;
  }
  return false;
}

export function routeMessageAdd(
  socketId: string,
  id: string,
  action: string,
  payload: Record<string, unknown>,
): boolean {
  if (action === "message.add" || action === "message.queue.add") {
    handleFollowUp(socketId, id, action, payload);
    return true;
  }
  return false;
}

export function routeMessageQueueGet(
  socketId: string,
  id: string,
  action: string,
  payload: Record<string, unknown>,
): boolean {
  if (action === "message.queue.get") {
    const session = state.sessions.find((candidate) => candidate.id === payload.session_id);
    if (!session || session.task_id !== payload.task_id) {
      // i18n-exempt: demo protocol fixtures retain backend wire errors and seeded prompts
      respond(socketId, id, { message: "Session not found" }, true);
      return true;
    }
    respond(socketId, id, demoQueueStatus(session));
    return true;
  }
  return false;
}

export function routeMessageSearch(
  socketId: string,
  id: string,
  action: string,
  payload: Record<string, unknown>,
): boolean {
  if (action === "message.search") {
    const query = String(payload.query || "").toLowerCase();
    const messages = state.messagesBySession[String(payload.session_id)] ?? [];
    const hits = messages
      .filter((message) => message.content.toLowerCase().includes(query))
      .map((message) => ({
        id: message.id,
        author_type: message.author_type,
        type: message.type,
        snippet: message.content,
        created_at: message.created_at,
      }));
    respond(socketId, id, { hits, total: hits.length });
    return true;
  }
  return false;
}
