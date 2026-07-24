"use client";

import { useCallback, useSyncExternalStore } from "react";
import { panelPortalManager } from "@/lib/layout/panel-portal-manager";

/**
 * Tracks whether the dockview panel identified by panelId is the active tab
 * in its dockview group. Portal-hosted panel content (see
 * `lib/layout/panel-portal-manager.ts`) stays mounted while its tab is
 * inactive — e.g. the Chat tab keeps running behind an active Files/Changes
 * tab — so components that must distinguish "mounted" from "actually on
 * screen" (like the unread-divider read tracker) need this signal rather
 * than assuming mount implies visibility.
 *
 * Backed by `panelPortalManager` since portal-hosted content only receives
 * its `panelId`, not the dockview api directly; `usePortalSlot`'s mount
 * effect acquires the portal entry (with its api) before any child content
 * renders, so the entry is virtually always present by the time this runs.
 * Subscribes to both the manager's own add/remove notifications (a panel
 * acquired after this hook's first mount, or released while it stays
 * mounted) and the panel's own `onDidActiveChange` (tab switches within an
 * existing group).
 *
 * Only for dockview-hosted panels — pass the panel's real id. Defaults to
 * `false` until a portal entry/api is actually registered for it, rather
 * than assuming visibility during the brief acquire/mount race or after a
 * panel has been detached: a false negative just delays marking the session
 * read by a render, while a false positive could mark it read too early.
 * Non-dockview hosts (mobile, quick chat, kanban preview) never acquire a
 * portal entry — they should pass a literal `true`/`false` instead of
 * calling this hook.
 */
export function usePanelActive(panelId: string): boolean {
  const subscribe = useCallback(
    (onStoreChange: () => void) => {
      const unsubscribeManager = panelPortalManager.subscribe(onStoreChange);
      const disposable = panelPortalManager.get(panelId)?.api?.onDidActiveChange(onStoreChange);
      return () => {
        unsubscribeManager();
        disposable?.dispose();
      };
    },
    [panelId],
  );
  const getSnapshot = useCallback(
    () => panelPortalManager.get(panelId)?.api?.isActive ?? false,
    [panelId],
  );
  return useSyncExternalStore(subscribe, getSnapshot, () => false);
}
