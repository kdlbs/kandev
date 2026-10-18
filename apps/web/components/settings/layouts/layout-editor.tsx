"use client";

import { useCallback, useEffect, useLayoutEffect, useRef, useState, type RefObject } from "react";
import {
  DockviewDefaultTab,
  DockviewReact,
  type DockviewApi,
  type DockviewReadyEvent,
  type IDockviewPanel,
  type IDockviewPanelHeaderProps,
  type IDockviewPanelProps,
} from "dockview-react";
import { themeKandev } from "@/lib/layout/dockview-theme";
import {
  fromDockviewApi,
  toSerializedDockview,
  type LayoutState,
} from "@/lib/state/layout-manager";
import { LayoutEditorActions, type LayoutEditorActionAnchor } from "./layout-editor-toolbar";
import {
  refreshCanonicalTitles,
  restoreCanonicalTitles,
  seedCanonicalTitles,
  useDisplayTitleReconciliation,
  type CanonicalTitles,
} from "./layout-editor-titles";

type LayoutEditorProps = {
  layout: LayoutState;
  editable: boolean;
  onChange: (layout: LayoutState) => void;
};

function PlaceholderPanel({ api }: IDockviewPanelProps) {
  return (
    <div
      className="flex h-full items-center justify-center bg-background/60 p-4 text-center text-sm text-muted-foreground"
      data-testid={`layout-placeholder-${api.id}`}
    >
      {api.title ?? api.id}
    </div>
  );
}

function EditorTab(props: IDockviewPanelHeaderProps) {
  return <DockviewDefaultTab {...props} hideClose />;
}

const placeholderComponents: Record<string, React.FunctionComponent<IDockviewPanelProps>> = {
  chat: PlaceholderPanel,
  files: PlaceholderPanel,
  changes: PlaceholderPanel,
  "pr-detail": PlaceholderPanel,
  terminal: PlaceholderPanel,
  plan: PlaceholderPanel,
  browser: PlaceholderPanel,
  vscode: PlaceholderPanel,
  // A saved preset can reference a plugin panel (Approach A1) — placeholder
  // so Settings > Layouts renders a generic box instead of throwing on an
  // unknown tabComponent (AC8), regardless of which plugin registered it.
  "plugin-panel": PlaceholderPanel,
  // Reusable panels rendered through the same placeholder box in the editor.
  todos: PlaceholderPanel,
};

const placeholderTabs = {
  permanentTab: EditorTab,
  changesTab: EditorTab,
  planTab: EditorTab,
  terminalTab: EditorTab,
  pluginPanelTab: EditorTab,
};

function activePanelId(panel: { id: string } | undefined, api: DockviewApi) {
  return panel?.id ?? api.panels[0]?.id ?? null;
}

function getActionAnchor(root: HTMLDivElement, selected: IDockviewPanel): LayoutEditorActionAnchor {
  const rootBounds = root.getBoundingClientRect();
  const groupBounds = selected.group.element.getBoundingClientRect();
  const groupTop = groupBounds.top - rootBounds.top;
  const groupBottom = groupBounds.bottom - rootBounds.top;
  const actionWidth = Math.min(360, rootBounds.width - 16);
  const centeredLeft = groupBounds.left - rootBounds.left + groupBounds.width / 2 - actionWidth / 2;
  return {
    left: Math.max(8, Math.min(centeredLeft, rootBounds.width - actionWidth - 8)),
    top: Math.max(groupTop + 8, Math.min(groupTop + 42, groupBottom - 54)),
    width: actionWidth,
  };
}

function EditorDock({
  rootRef,
  editable,
  onReady,
}: {
  rootRef: RefObject<HTMLDivElement | null>;
  editable: boolean;
  onReady: (event: DockviewReadyEvent) => void;
}) {
  return (
    <div ref={rootRef} className="h-[28rem] min-h-80 w-full min-w-0 overflow-hidden sm:h-[32rem]">
      <DockviewReact
        theme={themeKandev}
        components={placeholderComponents}
        tabComponents={placeholderTabs}
        defaultTabComponent={EditorTab}
        onReady={onReady}
        disableFloatingGroups
        disableDnd={!editable}
        locked={!editable}
        noPanelsOverlay="emptyGroup"
        className="h-full min-w-0"
      />
    </div>
  );
}

/** Guard that suppresses layout-change capture while the preview is mutated programmatically. */
function useApplyingGuard() {
  const applyingRef = useRef(false);
  const releaseTimerRef = useRef<ReturnType<typeof setTimeout> | null>(null);
  const holdApplying = useCallback(() => {
    applyingRef.current = true;
    if (releaseTimerRef.current) clearTimeout(releaseTimerRef.current);
    releaseTimerRef.current = setTimeout(() => {
      applyingRef.current = false;
    }, 0);
  }, []);
  const cancelRelease = useCallback(() => {
    if (releaseTimerRef.current) clearTimeout(releaseTimerRef.current);
  }, []);
  return { applyingRef, holdApplying, cancelRelease };
}

type ResizeSync = {
  api: DockviewApi | null;
  rootRef: RefObject<HTMLDivElement | null>;
  apiRef: RefObject<DockviewApi | null>;
  holdApplying: () => void;
  cancelRelease: () => void;
  onResized: () => void;
};

/** Keep Dockview's grid in step with the preview host's size without capturing it as an edit. */
function useResizeSync({
  api,
  rootRef,
  apiRef,
  holdApplying,
  cancelRelease,
  onResized,
}: ResizeSync) {
  useEffect(() => {
    const element = rootRef.current;
    if (!api || !element || typeof ResizeObserver === "undefined") return;
    const observer = new ResizeObserver(([entry]) => {
      const width = Math.round(entry.contentRect.width);
      const height = Math.round(entry.contentRect.height);
      if (width <= 0 || height <= 0) return;
      holdApplying();
      api.layout(width, height, true);
      onResized();
    });
    observer.observe(element);
    return () => {
      observer.disconnect();
      cancelRelease();
      apiRef.current = null;
    };
  }, [api, rootRef, apiRef, holdApplying, cancelRelease, onResized]);
}

export function LayoutEditor({ layout, editable, onChange }: LayoutEditorProps) {
  const rootRef = useRef<HTMLDivElement>(null);
  const apiRef = useRef<DockviewApi | null>(null);
  const canonicalTitlesRef = useRef<CanonicalTitles>(new Map());
  const { applyingRef, holdApplying, cancelRelease } = useApplyingGuard();
  const [api, setApi] = useState<DockviewApi | null>(null);
  const [selectedPanelId, setSelectedPanelId] = useState<string | null>(null);
  const [revision, setRevision] = useState(0);
  const [actionAnchor, setActionAnchor] = useState<LayoutEditorActionAnchor | null>(null);
  const bumpRevision = useCallback(() => setRevision((value) => value + 1), []);

  const updateActionAnchor = useCallback(() => {
    const root = rootRef.current;
    const selected = selectedPanelId ? apiRef.current?.getPanel(selectedPanelId) : null;
    if (!root || !selected) {
      setActionAnchor(null);
      return;
    }
    setActionAnchor(getActionAnchor(root, selected));
  }, [selectedPanelId]);

  useLayoutEffect(() => {
    updateActionAnchor();
  }, [revision, updateActionAnchor]);

  const capture = useCallback(() => {
    const current = apiRef.current;
    if (!current || applyingRef.current || !editable) return;
    refreshCanonicalTitles(current, canonicalTitlesRef.current);
    const captured = restoreCanonicalTitles(fromDockviewApi(current), canonicalTitlesRef.current);
    if (captured.columns.length > 0) onChange(captured);
    setRevision((revision) => revision + 1);
  }, [editable, onChange]);

  const apply = useCallback(
    (current: DockviewApi, nextLayout: LayoutState) => {
      const bounds = rootRef.current?.getBoundingClientRect();
      const width = Math.max(320, Math.round(bounds?.width ?? current.width ?? 900));
      const height = Math.max(320, Math.round(bounds?.height ?? current.height ?? 480));
      applyingRef.current = true;
      seedCanonicalTitles(nextLayout, canonicalTitlesRef.current);
      current.fromJSON(toSerializedDockview(nextLayout, width, height, new Map()));
      current.layout(width, height, true);
      setSelectedPanelId(current.activePanel?.id ?? current.panels[0]?.id ?? null);
      setRevision((revision) => revision + 1);
      holdApplying();
    },
    [holdApplying],
  );

  const onReady = useCallback(
    (event: DockviewReadyEvent) => {
      apiRef.current = event.api;
      setApi(event.api);
      apply(event.api, layout);
    },
    [apply, layout],
  );

  useEffect(() => {
    if (!api) return;
    const layoutDisposable = api.onDidLayoutChange(capture);
    const activeDisposable = api.onDidActivePanelChange((panel) => {
      setSelectedPanelId(activePanelId(panel, api));
      setRevision((revision) => revision + 1);
    });
    return () => {
      layoutDisposable.dispose();
      activeDisposable.dispose();
    };
  }, [api, capture]);

  useResizeSync({ api, rootRef, apiRef, holdApplying, cancelRelease, onResized: bumpRevision });

  useDisplayTitleReconciliation({
    api,
    titles: canonicalTitlesRef.current,
    applyingRef,
    holdApplying,
    onChanged: bumpRevision,
  });

  return (
    <div className="relative min-w-0 overflow-hidden rounded-md border" data-testid="layout-editor">
      <LayoutEditorActions
        api={api}
        anchor={actionAnchor}
        editable={editable}
        selectedPanelId={selectedPanelId}
        onCommand={capture}
      />
      <EditorDock rootRef={rootRef} editable={editable} onReady={onReady} />
    </div>
  );
}
