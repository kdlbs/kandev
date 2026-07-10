"use client";

import React, { useCallback, useEffect, useRef, memo } from "react";
import {
  DockviewReact,
  DockviewDefaultTab,
  type IDockviewPanelProps,
  type IDockviewPanelHeaderProps,
  type DockviewReadyEvent,
} from "dockview-react";
import { themeKandev } from "@/lib/layout/dockview-theme";
import { useDockviewStore, performLayoutSwitch } from "@/lib/state/dockview-store";
import { restoreEnvLayout } from "./dockview-layout-restore";
import {
  setupContainerResizeSync,
  setupGroupTracking,
  setupLayoutPersistence,
  setupPortalCleanup,
  setupSashDragCapToggle,
} from "./dockview-layout-setup";
import { useAppStore, useAppStoreApi } from "@/components/state-provider";
import { useLspFileOpener } from "@/hooks/use-lsp-file-opener";
import { useEditorKeybinds } from "@/hooks/use-editor-keybinds";
import { usePlanPanelAutoOpen } from "@/hooks/use-plan-panel-auto-open";

// Panel components (rendered via portals, not directly by dockview)
import { LeftHeaderActions, RightHeaderActions } from "./dockview-header-actions";
import { DockviewWatermark } from "./dockview-watermark";
import { ContextMenuTab } from "./tab-context-menu";
import { ChangesTab } from "./changes-tab";
import { useChangesPanelAutoFocus } from "./changes-panel-focus";
import { PlanTab } from "./plan-tab";
import { PreviewFileTab, PreviewDiffTab, PreviewCommitTab, PinnedDefaultTab } from "./preview-tab";
import { SessionTab } from "./session-tab";
import { TerminalTab } from "./terminal-tab";
import { useTabMaximizeOnDoubleClick } from "./use-tab-maximize";
import { setupSessionTabSync } from "./dockview-session-tab-sync";
import {
  setupChatPanelSafetyNet,
  useAutoSessionTab,
  useAutoPRPanel,
} from "./dockview-session-tabs";
import {
  useCompactDockviewDefault,
  useDockviewUnmountCleanup,
} from "./dockview-desktop-layout-hooks";
import { renderPanel } from "./dockview-panel-content";
import { PreviewController } from "./preview-controller";
import { BottomTerminalPanel } from "./bottom-terminal-panel";
import { DockviewReviewDialog } from "./dockview-review-dialog";
import { useReviewDialog } from "./use-review-dialog";

import type { Repository, RepositoryScript } from "@/lib/types/http";
import type { Terminal } from "@/hooks/domains/session/use-terminals";

// Portal system
import { PanelPortalHost, usePortalSlot } from "@/lib/layout/panel-portal-host";
import { ENV_SCOPED_DOCKVIEW_COMPONENTS } from "@/lib/state/dockview-env-scoped-components";
import { normalizeReusableSessionPanels, type LayoutState } from "@/lib/state/layout-manager";

// ---------------------------------------------------------------------------
// PORTAL SLOT — generic dockview component that adopts a persistent portal
// ---------------------------------------------------------------------------

/**
 * Components whose portals are tied to a specific task environment.
 *
 * When the user switches task envs, portals for these components are released
 * via `panelPortalManager.releaseByEnv()` so stale state (WebSocket
 * connections, iframes, editor buffers) from the old env doesn't leak
 * into the new one. Same-env session switches are a no-op — these portals
 * persist because the underlying workspace, container, and git history all
 * belong to the env, not the session.
 *
 * A component belongs here if its content is bound to env-specific runtime
 * state that can't be swapped by simply reading a new `activeSessionId` from
 * the store:
 *
 *  - file-editor   — editing a file in the env's worktree
 *  - browser       — iframe preview of the env's dev server URL
 *  - vscode        — VS Code Server iframe running in the env's container
 *  - commit-detail — displays a commit from the env's git history
 *  - diff-viewer   — shows file diffs from the env's working tree
 *  - pr-detail     — PR linked to the env's task
 *
 * Components NOT listed here are **global** — they read `activeSessionId`
 * reactively from the store and automatically reflect the current session:
 *
 *  - sidebar  — workspace/task navigation, not session-specific
 *  - chat     — subscribes to `activeSessionId`, re-renders for new session
 *  - terminal — uses `useEnvironmentSessionId()`, reconnects only on env change
 *  - changes  — uses `useEnvironmentSessionId()` for stable git state
 *  - files    — uses `useEnvironmentSessionId()` for stable file tree
 *  - plan     — reads `activeTaskId` from the store
 */
const ENV_SCOPED_COMPONENTS = ENV_SCOPED_DOCKVIEW_COMPONENTS;

/**
 * Every entry in the dockview `components` map uses this wrapper.
 * It renders an empty container and attaches the persistent portal element
 * managed by PanelPortalManager.  The actual panel content is rendered by
 * PanelPortalHost outside the dockview tree.
 *
 * Env-scoped panels are tagged with the current task-env ID so they can be
 * cleaned up on env switch.
 */
function PortalSlot(props: IDockviewPanelProps) {
  const component = props.api.component;
  const activeEnvId = useAppStore((s) => {
    const sid = s.tasks.activeSessionId;
    return sid ? (s.environmentIdBySessionId[sid] ?? null) : null;
  });
  const envId = ENV_SCOPED_COMPONENTS.has(component) ? (activeEnvId ?? undefined) : undefined;
  const containerRef = usePortalSlot(props, envId);
  return <div ref={containerRef} className="h-full w-full overflow-hidden" />;
}

// --- COMPONENT MAP ---
// All panel types use the same PortalSlot wrapper — dockview only manages
// layout positioning.  Actual rendering happens in PanelPortalHost below.
const components: Record<string, React.FunctionComponent<IDockviewPanelProps>> = {
  chat: PortalSlot,
  "diff-viewer": PortalSlot,
  "file-editor": PortalSlot,
  "commit-detail": PortalSlot,
  changes: PortalSlot,
  files: PortalSlot,
  terminal: PortalSlot,
  browser: PortalSlot,
  vscode: PortalSlot,
  plan: PortalSlot,
  "pr-detail": PortalSlot,
  // Backwards compat aliases for saved layouts
  "diff-files": PortalSlot,
  "all-files": PortalSlot,
};

// --- TAB COMPONENTS ---
function PermanentTab(props: IDockviewPanelHeaderProps) {
  const onDoubleClick = useTabMaximizeOnDoubleClick(props.api);
  return (
    <div
      className="flex h-full items-center cursor-pointer select-none"
      onDoubleClick={onDoubleClick}
    >
      <DockviewDefaultTab {...props} hideClose />
    </div>
  );
}

/** Sync the user's default saved layout from settings into the dockview store. */
function useSyncUserDefaultLayout() {
  const savedLayouts = useAppStore((s) => s.userSettings.savedLayouts);
  const setUserDefaultLayout = useDockviewStore((s) => s.setUserDefaultLayout);
  useEffect(() => {
    const defaultLayout = savedLayouts.find((l) => l.is_default);
    const state = defaultLayout?.layout as unknown as LayoutState | undefined;
    // Drop the obsolete "sidebar" column: the dockview-embedded sidebar pane was
    // retired for the unified AppSidebar, but a default layout saved before that
    // change still carries it. The default-build path applies this layout
    // without the restore-time sanitize layer, so an orphaned sidebar column
    // (its panel component is no longer registered) renders a broken grid.
    const columns = state?.columns?.filter((c) => c.id !== "sidebar");
    setUserDefaultLayout(
      columns && columns.length > 0 ? normalizeReusableSessionPanels({ ...state, columns }) : null,
    );
  }, [savedLayouts, setUserDefaultLayout]);
}

const tabComponents: Record<string, React.FunctionComponent<IDockviewPanelHeaderProps>> = {
  permanentTab: PermanentTab,
  changesTab: ChangesTab,
  planTab: PlanTab,
  sessionTab: SessionTab,
  terminalTab: TerminalTab,
  previewFileTab: PreviewFileTab,
  previewDiffTab: PreviewDiffTab,
  previewCommitTab: PreviewCommitTab,
  pinnedDefaultTab: PinnedDefaultTab,
};

// ---------------------------------------------------------------------------
// LAYOUT RESTORATION HELPERS
// ---------------------------------------------------------------------------

const VALID_COMPONENTS = new Set(Object.keys(components));

// ---------------------------------------------------------------------------
// useEnvSwitchCleanup — backup layout switch for external session changes
// ---------------------------------------------------------------------------

function useEnvSwitchCleanup(
  effectiveSessionId: string | null,
  effectiveEnvId: string | null,
  activeTaskId: string | null,
) {
  const prevEnvRef = useRef<string | null | undefined>(undefined);
  const prevTaskRef = useRef<string | null | undefined>(undefined);
  const currentSessionIdsKey = useAppStore((state) => {
    if (!activeTaskId) return "";
    return (state.taskSessionsByTask.itemsByTaskId[activeTaskId] ?? [])
      .map((session) => session.id)
      .join(",");
  });
  useEffect(() => {
    const newEnvId = effectiveEnvId;
    const newTaskId = activeTaskId;
    if (prevEnvRef.current === undefined) {
      prevEnvRef.current = newEnvId;
      prevTaskRef.current = newTaskId;
      return;
    }
    if (prevEnvRef.current === newEnvId) {
      prevTaskRef.current = newTaskId;
      return;
    }

    // Every session of a task shares ONE task_environment_id by design — the
    // backend reuses the task's environment for each session it launches (see
    // assignLaunchTaskEnvironmentID). A same-task env-id *change* between two
    // real envs therefore only happens via a launch race, and acting on it is
    // destructive: performEnvSwitch rebuilds the env-keyed layout and strips
    // the sibling session's chat panel (keepSessionId). With the active session
    // bouncing between the two envs the session tabs are repeatedly removed and
    // re-added — the "flicker between the old and new session". Sessions of one
    // task render in one shared layout, so keep the current layout and do NOT
    // advance prevEnvRef (preserve the task's stable env for a later real
    // task switch).
    const taskChanged = prevTaskRef.current !== newTaskId;
    if (!taskChanged && prevEnvRef.current && newEnvId) {
      return;
    }

    const oldEnvId = prevEnvRef.current;
    prevEnvRef.current = newEnvId;
    prevTaskRef.current = newTaskId;

    // Portal cleanup is handled synchronously inside switchEnvLayout (in the
    // dockview store action) before any fromJSON call. This hook serves as a
    // backup for external session changes (e.g. WS-driven) that don't go
    // through the sidebar/dropdown switch helpers. Same-env switches return
    // early above (no-op).
    if (newEnvId) {
      const currentSessionIds = currentSessionIdsKey ? currentSessionIdsKey.split(",") : [];
      if (effectiveSessionId && !currentSessionIds.includes(effectiveSessionId)) {
        currentSessionIds.unshift(effectiveSessionId);
      }
      performLayoutSwitch(oldEnvId, newEnvId, effectiveSessionId, currentSessionIds);
    }
  }, [effectiveEnvId, effectiveSessionId, activeTaskId, currentSessionIdsKey]);
}

// ---------------------------------------------------------------------------
// MAIN LAYOUT COMPONENT
// ---------------------------------------------------------------------------

type DockviewDesktopLayoutProps = {
  workspaceId: string | null;
  workflowId: string | null;
  sessionId?: string | null;
  repository?: Repository | null;
  initialScripts?: RepositoryScript[];
  initialTerminals?: Terminal[];
  initialLayout?: string | null;
  compact?: boolean;
};

type ReadyDockviewLayoutSetup = {
  buildDefaultLayout: (api: DockviewReadyEvent["api"], intentName?: string) => void;
  compact: boolean;
  initialLayout?: string | null;
};

type ReadyDockviewRefs = {
  envIdRef: React.MutableRefObject<string | null>;
  readyDisposersRef: React.MutableRefObject<Array<() => void>>;
  saveTimerRef: React.MutableRefObject<ReturnType<typeof setTimeout> | null>;
  setApi: (api: DockviewReadyEvent["api"] | null) => void;
};

type ReadyDockviewSetup = {
  api: DockviewReadyEvent["api"];
  appStore: ReturnType<typeof useAppStoreApi>;
  layout: ReadyDockviewLayoutSetup;
  refs: ReadyDockviewRefs;
};

function setupReadyDockview({ api, appStore, layout, refs }: ReadyDockviewSetup): void {
  refs.setApi(api);

  const currentEnvId = refs.envIdRef.current;
  const restored =
    !layout.initialLayout && restoreEnvLayout(api, currentEnvId, appStore, VALID_COMPONENTS);
  if (!restored) {
    layout.buildDefaultLayout(
      api,
      layout.initialLayout ?? (layout.compact ? "compact" : undefined),
    );
  }

  useDockviewStore.setState({ currentLayoutEnvId: currentEnvId });

  refs.readyDisposersRef.current.push(setupGroupTracking(api));
  const sessionTabSyncDisposable = setupSessionTabSync(api, appStore);
  refs.readyDisposersRef.current.push(() => sessionTabSyncDisposable.dispose());
  const chatPanelSafetyNetDisposable = setupChatPanelSafetyNet(api, appStore);
  refs.readyDisposersRef.current.push(() => chatPanelSafetyNetDisposable.dispose());
  refs.readyDisposersRef.current.push(
    setupLayoutPersistence(api, refs.saveTimerRef, refs.envIdRef),
  );
  setupPortalCleanup(api, appStore);
  refs.readyDisposersRef.current.push(setupContainerResizeSync(api));
  refs.readyDisposersRef.current.push(setupSashDragCapToggle(api));
}

type DockviewMainAreaProps = {
  effectiveSessionId: string | null;
  hasDevScript: boolean;
  onReady: (event: DockviewReadyEvent) => void;
};

function DockviewMainArea({ effectiveSessionId, hasDevScript, onReady }: DockviewMainAreaProps) {
  return (
    <div className="min-h-0 min-w-0 overflow-hidden flex flex-col">
      <PreviewController sessionId={effectiveSessionId} hasDevScript={hasDevScript} />
      <DockviewReact
        theme={themeKandev}
        components={components}
        tabComponents={tabComponents}
        defaultTabComponent={ContextMenuTab}
        leftHeaderActionsComponent={LeftHeaderActions}
        rightHeaderActionsComponent={RightHeaderActions}
        watermarkComponent={DockviewWatermark}
        onReady={onReady}
        defaultRenderer="always"
        className="flex-1 min-h-0"
      />
    </div>
  );
}

export const DockviewDesktopLayout = memo(function DockviewDesktopLayout({
  sessionId,
  repository,
  initialLayout,
  compact = false,
}: DockviewDesktopLayoutProps) {
  const setApi = useDockviewStore((s) => s.setApi);
  const buildDefaultLayout = useDockviewStore((s) => s.buildDefaultLayout);
  const saveTimerRef = useRef<ReturnType<typeof setTimeout> | null>(null);
  const readyDisposersRef = useRef<Array<() => void>>([]);
  const appStore = useAppStoreApi();

  const effectiveSessionId =
    useAppStore((state) => state.tasks.activeSessionId) ?? sessionId ?? null;
  const activeTaskId = useAppStore((state) => state.tasks.activeTaskId);
  const effectiveEnvId = useAppStore((state) =>
    effectiveSessionId ? (state.environmentIdBySessionId[effectiveSessionId] ?? null) : null,
  );
  const changesFocusKey = effectiveEnvId ?? effectiveSessionId;
  const envIdRef = useRef<string | null>(effectiveEnvId);
  const hasDevScript = Boolean(repository?.dev_script?.trim());

  const review = useReviewDialog(effectiveSessionId);

  useSyncUserDefaultLayout();
  useLspFileOpener();
  useEditorKeybinds();
  usePlanPanelAutoOpen();
  useCompactDockviewDefault(compact);

  useEffect(() => {
    envIdRef.current = effectiveEnvId;
  }, [effectiveEnvId]);

  const onReady = useCallback(
    (event: DockviewReadyEvent) => {
      setupReadyDockview({
        api: event.api,
        appStore,
        layout: { buildDefaultLayout, compact, initialLayout },
        refs: { envIdRef, readyDisposersRef, saveTimerRef, setApi },
      });
    },
    [setApi, buildDefaultLayout, initialLayout, compact, appStore],
  );

  // Release session-scoped portals + trigger layout switch on session change.
  // IMPORTANT: this must run BEFORE useAutoSessionTab so the old layout is
  // saved before a new session tab is created — otherwise the new session's
  // panel could leak into the old session's persisted layout.
  useEnvSwitchCleanup(effectiveSessionId, effectiveEnvId, activeTaskId);

  useAutoSessionTab(effectiveSessionId);
  useChangesPanelAutoFocus(changesFocusKey);

  useAutoPRPanel();
  useDockviewUnmountCleanup(saveTimerRef, readyDisposersRef);

  // Visual masking: hide the dockview container during slow-path layout
  // switches (full fromJSON rebuild) to prevent the old layout from flashing.
  const isRestoringLayout = useDockviewStore((s) => s.isRestoringLayout);

  return (
    <div
      data-testid="dockview-task-layout"
      className="flex-1 min-h-0 grid grid-rows-[1fr_auto]"
      aria-busy={isRestoringLayout}
      style={{
        opacity: isRestoringLayout ? 0 : 1,
        pointerEvents: isRestoringLayout ? "none" : undefined,
        transition: isRestoringLayout ? "none" : "opacity 60ms ease-out",
      }}
    >
      <DockviewMainArea
        effectiveSessionId={effectiveSessionId}
        hasDevScript={hasDevScript}
        onReady={onReady}
      />
      <BottomTerminalPanel />
      <PanelPortalHost renderPanel={renderPanel} />
      <DockviewReviewDialog sessionId={effectiveSessionId} review={review} />
    </div>
  );
});
