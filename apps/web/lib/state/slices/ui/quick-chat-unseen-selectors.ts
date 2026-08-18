import type { AppState } from "@/lib/state/store";

const EMPTY_UNSEEN_IDLE: Record<string, true> = {};

/** Reports whether the requested workspace has any unseen quick-chat idle markers. */
export function selectQuickChatHasUnseenIdle(
  state: AppState,
  workspaceId: string | null | undefined,
): boolean {
  if (!workspaceId) return false;
  return (
    Object.keys(state.quickChat?.unseenIdleByWorkspace?.[workspaceId] ?? EMPTY_UNSEEN_IDLE).length >
    0
  );
}
