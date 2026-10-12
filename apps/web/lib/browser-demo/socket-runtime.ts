import { respond } from "./runtime-state";
import {
  routeAutomationList,
  routeAutomationRunsListWorkspace,
  routeAutomationSummaries,
  routeAutomationSummary,
  routeConversationResponse,
  routeTaskRead,
} from "./socket-info-runtime";
import {
  routeMessageAdd,
  routeMessageList,
  routeMessageQueueGet,
  routeMessageSearch,
} from "./socket-message-runtime";
import {
  routePermissionRespond,
  routeSessionCumulativeDiff,
  routeSessionFileReviewGet,
  routeSessionFileReviewReset,
  routeSessionFileReviewUpdate,
  routeSessionGitCommits,
  routeTaskPlan,
} from "./socket-review-runtime";
import {
  routeSessionEnsure,
  routeSessionFocus,
  routeSessionStop,
  routeSessionSubscribe,
  routeSessionUnfocus,
  routeTaskSubscribe,
} from "./socket-session-runtime";
import {
  routeUserShellCreate,
  routeUserShellDestroy,
  routeUserShellList,
  routeWorkspaceFileCreate,
  routeWorkspaceFileDelete,
  routeWorkspaceFileGet,
  routeWorkspaceFileRename,
  routeWorkspaceFilesSearch,
  routeWorkspaceFileUpdate,
  routeWorkspaceTreeGet,
} from "./socket-workspace-runtime";

// i18n-exempt: browser demo fixture data is intentionally literal demo content
export function handleSocketRequest(socketId: string, raw: string) {
  let request: { id?: string; action?: string; payload?: Record<string, unknown> };
  try {
    request = JSON.parse(raw);
  } catch {
    return;
  }
  if (!request || typeof request !== "object") return;
  const id = request.id;
  const action = request.action ?? "";
  const payload = request.payload ?? {};
  if (!id) return;
  try {
    routeSocketRequest(socketId, id, action, payload);
  } catch (error) {
    respond(
      socketId,
      id,
      { message: error instanceof Error ? error.message : String(error), demo_mode: true },
      true,
    );
  }
}

export function routeSocketRequest(
  socketId: string,
  id: string,
  action: string,
  payload: Record<string, unknown>,
) {
  const handlers = [
    routeTaskSubscribe,
    routeSessionStop,
    routeTaskRead,
    routeConversationResponse,
    routeAutomationList,
    routeAutomationSummaries,
    routeAutomationSummary,
    routeAutomationRunsListWorkspace,
    routePermissionRespond,
    routeSessionSubscribe,
    routeSessionFocus,
    routeSessionUnfocus,
    routeTaskPlan,
    routeSessionGitCommits,
    routeSessionCumulativeDiff,
    routeSessionFileReviewGet,
    routeSessionFileReviewUpdate,
    routeSessionFileReviewReset,
    routeWorkspaceTreeGet,
    routeWorkspaceFileGet,
    routeWorkspaceFilesSearch,
    routeWorkspaceFileUpdate,
    routeWorkspaceFileCreate,
    routeWorkspaceFileDelete,
    routeWorkspaceFileRename,
    routeUserShellList,
    routeUserShellCreate,
    routeUserShellDestroy,
    routeMessageList,
    routeMessageAdd,
    routeMessageQueueGet,
    routeMessageSearch,
    routeSessionEnsure,
  ];
  if (handlers.some((handler) => handler(socketId, id, action, payload))) return;
  // i18n-exempt: demo protocol fixtures retain backend wire errors and seeded prompts
  respond(socketId, id, { message: `Unsupported demo action: ${action}`, demo_mode: true }, true);
}
