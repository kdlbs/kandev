import { demoGitData } from "./git-runtime";
import { conversationRuntime, respond, state } from "./runtime-state";
import { routeDemoTaskRead } from "./task-read-runtime";

export function routeTaskRead(
  socketId: string,
  id: string,
  action: string,
  payload: Record<string, unknown>,
): boolean {
  const taskRead = routeDemoTaskRead(
    state,
    action,
    payload,
    (sessionId) => demoGitData(sessionId).status.files,
  );
  if (taskRead) {
    respond(socketId, id, taskRead.payload, taskRead.error);
    return true;
  }
  return false;
}

export function routeConversationResponse(
  socketId: string,
  id: string,
  action: string,
  payload: Record<string, unknown>,
): boolean {
  const conversationResponse = conversationRuntime.route(socketId, action, payload);
  if (conversationResponse) {
    respond(socketId, id, conversationResponse);
    return true;
  }
  return false;
}

export function routeAutomationList(
  socketId: string,
  id: string,
  action: string,
  _payload: Record<string, unknown>,
): boolean {
  if (
    action === "automation.list" ||
    action === "automation.runs.list" ||
    action === "automation.trigger_types"
  ) {
    respond(socketId, id, []);
    return true;
  }
  return false;
}

export function routeAutomationSummaries(
  socketId: string,
  id: string,
  action: string,
  _payload: Record<string, unknown>,
): boolean {
  if (action === "automation.summaries") {
    respond(socketId, id, { summaries: [] });
    return true;
  }
  return false;
}

export function routeAutomationSummary(
  socketId: string,
  id: string,
  action: string,
  _payload: Record<string, unknown>,
): boolean {
  if (action === "automation.summary") {
    respond(socketId, id, { summary: null });
    return true;
  }
  return false;
}

export function routeAutomationRunsListWorkspace(
  socketId: string,
  id: string,
  action: string,
  _payload: Record<string, unknown>,
): boolean {
  if (action === "automation.runs.list_workspace") {
    respond(socketId, id, { runs: [] });
    return true;
  }
  return false;
}
