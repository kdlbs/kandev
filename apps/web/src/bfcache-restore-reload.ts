const PAGESHOW_EVENT = "pageshow";
const RESTORE_PROBE_KEY = "kandev.bfcacheRestoreProbe";

type RestoreReloadTarget = Pick<EventTarget, "addEventListener" | "removeEventListener">;

interface RestoreReloadOptions {
  target?: RestoreReloadTarget;
  reload?: () => void;
  isDebug?: () => boolean;
  getStorage?: () => Pick<Storage, "setItem">;
  getNavigationType?: () => string | undefined;
}

function readNavigationType(): string | undefined {
  try {
    const entry = performance.getEntriesByType("navigation")[0];
    return entry ? (entry as PerformanceNavigationTiming).type : undefined;
  } catch {
    return undefined;
  }
}

/**
 * Reloads the page when it is restored from a frozen browser snapshot
 * (back/forward cache restore, including Chrome's "Duplicate tab") instead of
 * being loaded fresh. A restored page keeps the frozen JS heap and DOM, so
 * state can be stale until a real load; the reload re-fetches the no-store
 * boot payload and reconnects the WebSocket.
 *
 * A restore is detected on `pageshow` when `event.persisted` is true. That is
 * the only reliable frozen-restore signal: the navigation type
 * `back_forward` also covers cold history traversals and session-restored
 * tabs, which load fresh and must NOT be reloaded a second time. Fresh loads
 * (`navigate`), manual refreshes (`reload`), and in-app SPA routing never
 * reload; a reload produces a fresh document and cannot loop.
 *
 * In debug builds (`window.__KANDEV_DEBUG`), restore evidence is written to
 * sessionStorage BEFORE the reload so a native Duplicate-tab run can be
 * verified after the fact: the reload destroys the document, and the
 * duplicated tab is a separate DevTools target, so the event data must be
 * persisted pre-reload. For duplicate/history candidates the actual
 * `persisted` value is recorded even when it is false, so the merge gate can
 * distinguish "no pageshow at all" from "pageshow with persisted=false". The
 * probe is diagnostics only, is skipped outside debug builds, and never
 * blocks the reload.
 *
 * Returns an uninstall function.
 */
export function installBfcacheRestoreReload({
  target = window,
  reload = () => window.location.reload(),
  isDebug = () => window.__KANDEV_DEBUG === true,
  getStorage = () => window.sessionStorage,
  getNavigationType = readNavigationType,
}: RestoreReloadOptions = {}): () => void {
  const recordProbe = (persisted: boolean) => {
    try {
      if (!isDebug()) return;
      const navigationType = getNavigationType();
      // Only duplicate/history candidates are probed: a frozen restore, or
      // any document reached via a `back_forward` traversal. Fresh loads and
      // manual refreshes are not candidates and are never recorded.
      if (!persisted && navigationType !== "back_forward") return;
      getStorage().setItem(
        RESTORE_PROBE_KEY,
        JSON.stringify({ persisted, navigationType, at: Date.now() }),
      );
    } catch {
      // Best-effort diagnostics; never block the reload.
    }
  };

  const handlePageshow = (event: Event) => {
    const persisted = (event as PageTransitionEvent).persisted === true;
    recordProbe(persisted);
    if (persisted) {
      reload();
    }
  };

  target.addEventListener(PAGESHOW_EVENT, handlePageshow);
  return () => target.removeEventListener(PAGESHOW_EVENT, handlePageshow);
}
