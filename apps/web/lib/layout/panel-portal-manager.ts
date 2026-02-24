/**
 * PanelPortalManager — singleton registry for persistent dockview panel portals.
 *
 * When dockview calls `api.fromJSON()` during layout switches, all React panel
 * components are unmounted and remounted.  This destroys expensive state such as
 * xterm WebSocket connections, iframes, and editor instances.
 *
 * The portal manager lifts panel content out of dockview's lifecycle:
 *  – Each panel gets a persistent `HTMLDivElement` (the "portal element") that
 *    lives in a hidden host outside the dockview tree.
 *  – Dockview panel wrappers adopt the portal element into their own DOM on mount
 *    and release it on unmount — without destroying it.
 *  – The actual React component tree is rendered into the portal element via
 *    `createPortal`, so component state, effects, and DOM survive layout switches.
 *
 * ## Session scoping
 *
 * Portals are either **global** or **session-scoped**:
 *
 * - **Global** (`sessionId` is `undefined`) — the portal persists across session
 *   switches. Used for panels that read `activeSessionId` reactively from the
 *   store and automatically show the correct data for whichever session is active.
 *   Examples: sidebar, chat, changes, files, plan.
 *
 * - **Session-scoped** (`sessionId` is set) — the portal is bound to the session
 *   that created it and is destroyed via `releaseBySession()` when the user
 *   switches away. Used for panels whose content is intrinsically tied to a
 *   specific session's runtime state (container processes, worktree files, etc.)
 *   and cannot simply re-read a store selector to switch context.
 *   Examples: terminal, file-editor, browser, vscode, commit-detail, diff-viewer,
 *   pr-detail.
 *
 * See `SESSION_SCOPED_COMPONENTS` in `dockview-desktop-layout.tsx` for the
 * authoritative list and per-component rationale.
 */

import type { DockviewPanelApi } from "dockview-react";

export type PortalEntry = {
  /** The persistent DOM element that React portals into. */
  element: HTMLDivElement;
  /** The dockview component name (e.g. "terminal", "chat"). */
  component: string;
  /** Latest panel params from dockview. */
  params: Record<string, unknown>;
  /** Latest dockview panel API handle — updated on each remount. */
  api: DockviewPanelApi | null;
  /** Session ID this portal is scoped to (undefined = global, persists across sessions). */
  sessionId?: string;
};

type Listener = () => void;

class PanelPortalManager {
  private entries = new Map<string, PortalEntry>();
  private listeners = new Set<Listener>();

  /**
   * Get or create the portal entry for a panel.
   * Called by the dockview slot wrapper on mount.
   */
  acquire(
    panelId: string,
    component: string,
    params: Record<string, unknown>,
    api: DockviewPanelApi,
    sessionId?: string,
  ): PortalEntry {
    let entry = this.entries.get(panelId);
    if (!entry) {
      const el = document.createElement("div");
      el.style.display = "contents";
      el.dataset.portalPanel = panelId;
      entry = { element: el, component, params, api, sessionId };
      this.entries.set(panelId, entry);
      this.notify();
    } else {
      // Panel remounted after fromJSON — update api & params
      entry.api = api;
      entry.params = params;
      entry.component = component;
    }
    return entry;
  }

  /** Remove a panel's portal (permanent deletion, e.g. user closes tab). */
  release(panelId: string): void {
    const entry = this.entries.get(panelId);
    if (!entry) return;
    entry.element.remove();
    entry.api = null;
    this.entries.delete(panelId);
    this.notify();
  }

  /** Release all portals scoped to a specific session. */
  releaseBySession(sessionId: string): void {
    const toRemove: string[] = [];
    for (const [panelId, entry] of this.entries) {
      if (entry.sessionId === sessionId) {
        toRemove.push(panelId);
      }
    }
    if (toRemove.length === 0) return;
    for (const panelId of toRemove) {
      const entry = this.entries.get(panelId)!;
      entry.element.remove();
      entry.api = null;
      this.entries.delete(panelId);
    }
    this.notify();
  }

  /** Release all portals (e.g. when the layout component unmounts entirely). */
  releaseAll(): void {
    if (this.entries.size === 0) return;
    for (const entry of this.entries.values()) {
      entry.element.remove();
      entry.api = null;
    }
    this.entries.clear();
    this.notify();
  }

  /** Check if a portal exists for a panel. */
  has(panelId: string): boolean {
    return this.entries.has(panelId);
  }

  /** Get a specific entry. */
  get(panelId: string): PortalEntry | undefined {
    return this.entries.get(panelId);
  }

  /** All registered panel IDs. */
  ids(): string[] {
    return Array.from(this.entries.keys());
  }

  /** Iterate all registered portals (used by the host to render). */
  getAll(): Map<string, PortalEntry> {
    return this.entries;
  }

  /** Subscribe to entry additions/removals. Returns unsubscribe fn. */
  subscribe(listener: Listener): () => void {
    this.listeners.add(listener);
    return () => this.listeners.delete(listener);
  }

  private notify(): void {
    for (const fn of this.listeners) fn();
  }
}

/** App-wide singleton. */
export const panelPortalManager = new PanelPortalManager();

/**
 * Set the dockview tab title for a panel managed by the portal system.
 * Safe to call even when the panel's api isn't available yet.
 */
export function setPanelTitle(panelId: string, title: string): void {
  const entry = panelPortalManager.get(panelId);
  if (entry?.api && entry.api.title !== title) {
    entry.api.setTitle(title);
  }
}
