"use client";

import {
  createContext,
  useCallback,
  useContext,
  useEffect,
  useRef,
  useState,
  type RefObject,
} from "react";
import {
  DockviewDefaultTab,
  DockviewReact,
  type DockviewApi,
  type DockviewReadyEvent,
  type IDockviewPanelHeaderProps,
  type IDockviewPanelProps,
} from "dockview-react";
import { themeKandev } from "@/lib/layout/dockview-theme";
import {
  fromDockviewApi,
  toSerializedDockview,
  type LayoutState,
} from "@/lib/state/layout-manager";
import { LayoutEditorToolbar } from "./layout-editor-toolbar";

type LayoutEditorProps = {
  layout: LayoutState;
  editable: boolean;
  onChange: (layout: LayoutState) => void;
};

const EditorContext = createContext({ editable: false });

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
  const { editable } = useContext(EditorContext);
  return <DockviewDefaultTab {...props} hideClose={!editable || props.api.id === "chat"} />;
}

const placeholderComponents = {
  chat: PlaceholderPanel,
  files: PlaceholderPanel,
  changes: PlaceholderPanel,
  terminal: PlaceholderPanel,
  plan: PlaceholderPanel,
  browser: PlaceholderPanel,
  vscode: PlaceholderPanel,
};

const placeholderTabs = {
  permanentTab: EditorTab,
  changesTab: EditorTab,
  planTab: EditorTab,
  terminalTab: EditorTab,
};

function activePanelId(panel: { id: string } | undefined, api: DockviewApi) {
  return panel?.id ?? api.panels[0]?.id ?? null;
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

export function LayoutEditor({ layout, editable, onChange }: LayoutEditorProps) {
  const rootRef = useRef<HTMLDivElement>(null);
  const apiRef = useRef<DockviewApi | null>(null);
  const applyingRef = useRef(false);
  const releaseTimerRef = useRef<ReturnType<typeof setTimeout> | null>(null);
  const [api, setApi] = useState<DockviewApi | null>(null);
  const [selectedPanelId, setSelectedPanelId] = useState<string | null>(null);
  const [, setRevision] = useState(0);

  const capture = useCallback(() => {
    const current = apiRef.current;
    if (!current || applyingRef.current || !editable) return;
    const captured = fromDockviewApi(current);
    if (captured.columns.length > 0) onChange(captured);
    setRevision((revision) => revision + 1);
  }, [editable, onChange]);

  const apply = useCallback((current: DockviewApi, nextLayout: LayoutState) => {
    const bounds = rootRef.current?.getBoundingClientRect();
    const width = Math.max(320, Math.round(bounds?.width ?? current.width ?? 900));
    const height = Math.max(320, Math.round(bounds?.height ?? current.height ?? 480));
    applyingRef.current = true;
    current.fromJSON(toSerializedDockview(nextLayout, width, height, new Map()));
    current.layout(width, height, true);
    setSelectedPanelId(current.activePanel?.id ?? current.panels[0]?.id ?? null);
    setRevision((revision) => revision + 1);
    if (releaseTimerRef.current) clearTimeout(releaseTimerRef.current);
    releaseTimerRef.current = setTimeout(() => {
      applyingRef.current = false;
    }, 0);
  }, []);

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

  useEffect(() => {
    const element = rootRef.current;
    if (!api || !element || typeof ResizeObserver === "undefined") return;
    const observer = new ResizeObserver(([entry]) => {
      const width = Math.round(entry.contentRect.width);
      const height = Math.round(entry.contentRect.height);
      if (width <= 0 || height <= 0) return;
      applyingRef.current = true;
      api.layout(width, height, true);
      if (releaseTimerRef.current) clearTimeout(releaseTimerRef.current);
      releaseTimerRef.current = setTimeout(() => {
        applyingRef.current = false;
      }, 0);
    });
    observer.observe(element);
    return () => {
      observer.disconnect();
      if (releaseTimerRef.current) clearTimeout(releaseTimerRef.current);
      apiRef.current = null;
    };
  }, [api]);

  return (
    <EditorContext.Provider value={{ editable }}>
      <div className="min-w-0 overflow-hidden rounded-md border" data-testid="layout-editor">
        <LayoutEditorToolbar
          api={api}
          editable={editable}
          selectedPanelId={selectedPanelId}
          onSelectedPanelChange={setSelectedPanelId}
          onCommand={capture}
        />
        <EditorDock rootRef={rootRef} editable={editable} onReady={onReady} />
      </div>
    </EditorContext.Provider>
  );
}
