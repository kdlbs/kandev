import { persistEnvLayoutNow, useDockviewStore } from "@/lib/state/dockview-store";

type ViewerReturn = { sessionId: string; path: string; repo?: string; expires: number };
let pending: ViewerReturn | null = null;

/** Retain navigation metadata only. A return never retains bytes or approval. */
export function rememberFileViewerReturn(sessionId: string, path: string, repo?: string) {
  const state = useDockviewStore.getState();
  if (state.api && !state.isRestoringLayout) {
    persistEnvLayoutNow(state.api, state.currentLayoutEnvId, state.preMaximizeLayout);
    return;
  }
  pending = { sessionId, path, repo, expires: Date.now() + 5 * 60_000 };
}

export function takeFileViewerReturn(sessionId: string) {
  const result = pending;
  if (!result) return null;
  if (result.expires < Date.now()) {
    pending = null;
    return null;
  }
  if (result.sessionId !== sessionId) return null;
  pending = null;
  return { path: result.path, repo: result.repo };
}
