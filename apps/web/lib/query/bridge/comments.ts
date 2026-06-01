import type { QueryClient } from "@tanstack/react-query";
import type { WebSocketClient } from "@/lib/ws/client";

/**
 * Registers the WS → TanStack Query bridge for the comments domain.
 *
 * WHY THIS IS A NO-OP:
 * Comments in this domain (diff, plan, file-editor, PR-feedback) are
 * purely client-side annotations. They are:
 *   1. Created locally by the user selecting code or plan text.
 *   2. Persisted to sessionStorage (lib/state/slices/comments/persistence.ts).
 *   3. Sent to the agent via useRunComment → message.add WS call.
 *   4. Removed from the store once marked "sent" (markCommentsSent).
 *
 * No WS event from the backend pushes comment data — the backend only
 * broadcasts "office.comment.created" for Office task-level user comments,
 * which belong to the Office domain and are handled by the office bridge.
 *
 * The "office.comment.created" event is handled in lib/query/bridge/office.ts,
 * which invalidates the office task comments cache (a different concept).
 *
 * If annotations are ever persisted server-side in the future, this bridge
 * would subscribe to events like "session.comment.added" and call:
 *   queryClient.setQueryData(qk.comments.bySession(sessionId), updater)
 */
export function registerCommentsBridge(
  _ws: WebSocketClient,
  _queryClient: QueryClient,
): () => void {
  // No-op: comments are client-side-only state with no server WS events.
  return () => {
    // No subscriptions to clean up.
  };
}
