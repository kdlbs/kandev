"use client";

import type { ReactNode, MouseEvent, RefObject } from "react";
import type { Layout } from "react-resizable-panels";
import { memo, useEffect, useCallback, useState, useMemo, useRef } from "react";
import { SessionPanel } from "@kandev/ui/pannel-session";
import { Group, Panel } from "react-resizable-panels";
import {
  getLocalStorage,
  setLocalStorage,
  getSessionStorage,
  setSessionStorage,
} from "@/lib/local-storage";
import { useAppStore } from "@/components/state-provider";
import { useLayoutStore } from "@/lib/state/layout-store";
import { useDefaultLayout } from "@/lib/layout/use-default-layout";
import { SessionTabs, type SessionTab } from "@/components/session-tabs";
import { useRepositoryScripts } from "@/hooks/domains/workspace/use-repository-scripts";
import { useTerminals } from "@/hooks/domains/session/use-terminals";
import { ParkedTerminalsMenu } from "@/components/task/parked-terminals-menu";
import { CloseTerminalConfirmPopover } from "@/components/task/close-terminal-confirm-popover";
import {
  CommandsTabContent,
  TerminalTabContents,
} from "@/components/task/task-right-panel-tab-contents";
import { TerminalTabMenu } from "@/components/task/terminal-tab";
import { TabRenameInput } from "@/components/task/tab-rename-input";
import type { RepositoryScript } from "@/lib/types/http";
import type { Terminal } from "@/hooks/domains/session/use-terminals";
import { useTranslation } from "react-i18next";

type TaskRightPanelProps = {
  topPanel: ReactNode;
  sessionId?: string | null;
  repositoryId?: string | null;
  initialScripts?: RepositoryScript[];
  initialTerminals?: Terminal[];
};

const DEFAULT_RIGHT_LAYOUT: Record<string, number> = { top: 55, bottom: 45 };

type TerminalTabsBuilderOpts = {
  terminals: Terminal[];
  handleCloseDevTab: (event: MouseEvent) => void;
  handleCloseTab: (event: MouseEvent, id: string) => void;
  renderContextMenu?: (
    terminal: Terminal,
    closeAnchorRef: RefObject<HTMLElement | null>,
  ) => ReactNode;
  renderContent?: (terminal: Terminal) => ReactNode;
  onDoubleClick?: (event: MouseEvent, terminal: Terminal) => void;
};

/**
 * Builds the SessionTab[] for the right-panel strip.
 *
 * Ordinary terminals get a `#N` badge (sequence number from the DB row) and
 * a context-menu hook for rename/destroy. Dev-server and script terminals
 * preserve their existing label/closable semantics.
 */
function buildTerminalTabs({
  terminals,
  handleCloseDevTab,
  handleCloseTab,
  renderContextMenu,
  renderContent,
  onDoubleClick,
}: TerminalTabsBuilderOpts): SessionTab[] {
  return terminals.map((terminal) => {
    const isOrdinary = terminal.kind === "ordinary";
    const isDev = terminal.type === "dev-server";
    return {
      id: terminal.id,
      label: terminal.label,
      badge: isOrdinary && terminal.seq ? `#${terminal.seq}` : undefined,
      truncate: !isOrdinary,
      className: terminal.closable ? "group flex items-center cursor-pointer" : "cursor-pointer",
      closable: terminal.closable,
      onClose: isDev ? handleCloseDevTab : (e: MouseEvent) => handleCloseTab(e, terminal.id),
      renderContextMenu:
        isOrdinary && renderContextMenu
          ? (closeAnchorRef) => renderContextMenu(terminal, closeAnchorRef)
          : undefined,
      content: isOrdinary ? renderContent?.(terminal) : undefined,
      onDoubleClick:
        isOrdinary && onDoubleClick ? (e: MouseEvent) => onDoubleClick(e, terminal) : undefined,
      testId: `terminal-tab-${terminal.id}`,
      closeTestId: `terminal-tab-close-${terminal.id}`,
    };
  });
}

function useRightPanelScripts(repositoryId: string | null, initialScripts: RepositoryScript[]) {
  const { scripts: storeScripts, isLoaded: scriptsLoaded } = useRepositoryScripts(repositoryId);
  const scripts = !scriptsLoaded && initialScripts.length > 0 ? initialScripts : storeScripts;
  return { scripts, hasScripts: scripts.length > 0 };
}

function useRightPanelPersistence({
  sessionId,
  hasScripts,
  activeTab,
  isBottomCollapsed,
  setRightPanelActiveTab,
}: {
  sessionId: string | null;
  hasScripts: boolean;
  activeTab: string | undefined;
  isBottomCollapsed: boolean;
  setRightPanelActiveTab: (sessionId: string, tabId: string) => void;
}) {
  useEffect(() => {
    if (!sessionId || !hasScripts) return;
    const savedTab = getSessionStorage<string | null>(`rightPanel-tab-${sessionId}`, null);
    if (savedTab === "commands") setRightPanelActiveTab(sessionId, savedTab);
  }, [sessionId, hasScripts, setRightPanelActiveTab]);

  useEffect(() => {
    if (!sessionId || !activeTab) return;
    setSessionStorage(`rightPanel-tab-${sessionId}`, activeTab);
  }, [sessionId, activeTab]);

  useEffect(() => {
    setLocalStorage("task-right-panel-collapsed", isBottomCollapsed);
  }, [isBottomCollapsed]);
}

function useRightPanelTabChange(
  sessionId: string | null,
  setRightPanelActiveTab: (sessionId: string, tabId: string) => void,
) {
  return useCallback(
    (value: string) => {
      if (sessionId) setRightPanelActiveTab(sessionId, value);
    },
    [sessionId, setRightPanelActiveTab],
  );
}

function useRightPanelTabs({
  hasScripts,
  terminals,
  handleCloseDevTab,
  handleCloseTab,
  renameTerminal,
  requestCloseFromContext,
  sessionId,
  setRightPanelActiveTab,
}: {
  hasScripts: boolean;
  terminals: Terminal[];
  handleCloseDevTab: (event: MouseEvent) => void;
  handleCloseTab: (event: MouseEvent, terminalId: string) => void;
  renameTerminal: (id: string, name: string | null) => Promise<void> | void;
  requestCloseFromContext: (id: string, closeAnchor: HTMLElement | null) => void;
  sessionId: string | null;
  setRightPanelActiveTab: (sessionId: string, tabId: string) => void;
}) {
  const { t } = useTranslation();
  const [renamingTerminalId, setRenamingTerminalId] = useState<string | null>(null);

  const handleStartRename = useCallback((terminalId: string) => {
    setRenamingTerminalId(terminalId);
  }, []);
  const handleRenameCommit = useCallback(
    async (terminalId: string, next: string) => {
      setRenamingTerminalId(null);
      const trimmed = next.trim();
      try {
        await renameTerminal(terminalId, trimmed === "" ? null : trimmed);
      } catch (error) {
        console.error(error);
      }
    },
    [renameTerminal],
  );
  const handleRenameCancel = useCallback(() => {
    setRenamingTerminalId(null);
  }, []);

  const renderTerminalContextMenu = useCallback(
    (terminal: Terminal, closeAnchorRef: RefObject<HTMLElement | null>) => (
      <TerminalTabMenu
        canMutate
        onStartRename={() => handleStartRename(terminal.id)}
        onClosePanel={() => undefined}
        onTerminatePanel={() => requestCloseFromContext(terminal.id, closeAnchorRef.current)}
      />
    ),
    [handleStartRename, requestCloseFromContext],
  );
  const renderTerminalContent = useCallback(
    (terminal: Terminal) => {
      if (renamingTerminalId !== terminal.id) return undefined;
      let initial = terminal.label;
      if (terminal.seq) initial = t("task:terminalWithSeq", { seq: terminal.seq });
      if (terminal.customName && terminal.customName !== "") initial = terminal.customName;
      return (
        <TabRenameInput
          initial={initial}
          seqBadge={terminal.seq ?? null}
          onCommit={(next) => void handleRenameCommit(terminal.id, next)}
          onCancel={handleRenameCancel}
          testId="terminal-tab-rename-input"
        />
      );
    },
    [handleRenameCancel, handleRenameCommit, renamingTerminalId, t],
  );
  const tabs: SessionTab[] = useMemo(() => {
    const commandsTabs: SessionTab[] = hasScripts
      ? [{ id: "commands", label: t("common:commandGroupCommands") }]
      : [];
    return [
      ...commandsTabs,
      ...buildTerminalTabs({
        terminals,
        handleCloseDevTab,
        handleCloseTab,
        renderContextMenu: renderTerminalContextMenu,
        renderContent: renderTerminalContent,
        onDoubleClick: (_event, terminal) => handleStartRename(terminal.id),
      }),
    ];
  }, [
    hasScripts,
    terminals,
    handleCloseDevTab,
    handleCloseTab,
    renderTerminalContextMenu,
    renderTerminalContent,
    handleStartRename,
    t,
  ]);
  const handleTabChange = useRightPanelTabChange(sessionId, setRightPanelActiveTab);

  return { tabs, handleTabChange };
}

/** Gate every destructive terminal close behind a control-anchored confirmation. */
function useConfirmableTerminalClose({
  terminals,
  handleCloseTab,
  destroyTerminal,
}: {
  terminals: Terminal[];
  handleCloseTab: (event: MouseEvent, terminalId: string) => void;
  destroyTerminal: (id: string) => Promise<boolean>;
}) {
  const [pendingClose, setPendingClose] = useState<Terminal | null>(null);
  const closeAnchorRef = useRef<HTMLElement>(null);

  const requestClose = useCallback(
    (terminalId: string, anchor: HTMLElement | null) => {
      const terminal = terminals.find((t) => t.id === terminalId);
      if (!terminal || !anchor) return false;
      closeAnchorRef.current = anchor;
      setPendingClose(terminal);
      return true;
    },
    [terminals],
  );

  const handleAskCloseTab = useCallback(
    (event: MouseEvent, terminalId: string) => {
      if (requestClose(terminalId, event.currentTarget as HTMLElement)) {
        event.preventDefault();
        event.stopPropagation();
        return;
      }
      handleCloseTab(event, terminalId);
    },
    [requestClose, handleCloseTab],
  );

  const handleConfirmClose = useCallback(async () => {
    if (!pendingClose) return;
    await destroyTerminal(pendingClose.id);
  }, [pendingClose, destroyTerminal]);

  return {
    pendingClose,
    setPendingClose,
    closeAnchorRef,
    handleAskCloseTab,
    requestClose,
    handleConfirmClose,
  };
}

type CollapsedRightPanelProps = {
  topPanel: ReactNode;
  tabs: SessionTab[];
  terminalTabValue: string;
  handleTabChange: (value: string) => void;
  addTerminal: () => void;
  onExpand: () => void;
};

function CollapsedRightPanel({
  topPanel,
  tabs,
  terminalTabValue,
  handleTabChange,
  addTerminal,
  onExpand,
}: CollapsedRightPanelProps) {
  return (
    <div className="h-full min-h-0 flex flex-col gap-1">
      <div className="flex-1 min-h-0">{topPanel}</div>
      <SessionPanel
        borderSide="left"
        className="!h-10 !p-0 mt-[2px] justify-between items-center flex-row"
      >
        <SessionTabs
          tabs={tabs}
          activeTab={terminalTabValue}
          onTabChange={handleTabChange}
          showAddButton
          onAddTab={addTerminal}
          collapsible
          isCollapsed={true}
          onToggleCollapse={onExpand}
          className="flex-1 min-h-0"
        />
      </SessionPanel>
    </div>
  );
}

type RightPanelContentProps = {
  isBottomCollapsed: boolean;
  topPanel: ReactNode;
  tabs: SessionTab[];
  terminalTabValue: string;
  handleTabChange: (value: string) => void;
  addTerminal: () => void;
  setIsBottomCollapsed: (v: boolean) => void;
  rightLayoutKey: string;
  rightLayout: Layout | undefined;
  onRightLayoutChange: (layout: Layout) => void;
  scripts: RepositoryScript[];
  handleRunCommand: (script: RepositoryScript) => void;
  terminals: Terminal[];
  parkedTerminals: Terminal[];
  resumeTerminal: (id: string) => Promise<void> | void;
  destroyTerminal: (id: string) => Promise<boolean> | void;
  environmentId: string | null;
  devProcessId: string | null | undefined;
  devOutput: string | undefined;
  isStoppingDev: boolean;
};

function RightPanelContent({
  isBottomCollapsed,
  topPanel,
  tabs,
  terminalTabValue,
  handleTabChange,
  addTerminal,
  setIsBottomCollapsed,
  rightLayoutKey,
  rightLayout,
  onRightLayoutChange,
  scripts,
  handleRunCommand,
  terminals,
  parkedTerminals,
  resumeTerminal,
  destroyTerminal,
  environmentId,
  devProcessId,
  devOutput,
  isStoppingDev,
}: RightPanelContentProps) {
  if (isBottomCollapsed) {
    return (
      <CollapsedRightPanel
        topPanel={topPanel}
        tabs={tabs}
        terminalTabValue={terminalTabValue}
        handleTabChange={handleTabChange}
        addTerminal={addTerminal}
        onExpand={() => setIsBottomCollapsed(false)}
      />
    );
  }

  return (
    <Group
      orientation="vertical"
      className="h-full min-h-0"
      id={rightLayoutKey}
      key={rightLayoutKey}
      defaultLayout={rightLayout}
      onLayoutChanged={onRightLayoutChange}
    >
      <Panel id="top" minSize={15} className="min-h-0">
        {topPanel}
      </Panel>
      <Panel id="bottom" minSize={15} className="min-h-0">
        <SessionPanel borderSide="left" margin="top">
          <SessionTabs
            tabs={tabs}
            activeTab={terminalTabValue}
            onTabChange={handleTabChange}
            showAddButton
            onAddTab={addTerminal}
            collapsible
            isCollapsed={isBottomCollapsed}
            onToggleCollapse={() => setIsBottomCollapsed(true)}
            rightContent={
              parkedTerminals.length > 0 ? (
                <ParkedTerminalsMenu
                  parkedTerminals={parkedTerminals}
                  onResume={resumeTerminal}
                  onDestroy={(terminalId) => {
                    void destroyTerminal(terminalId);
                  }}
                />
              ) : undefined
            }
            className="flex-1 min-h-0"
          >
            <CommandsTabContent scripts={scripts} onRunCommand={handleRunCommand} />
            <TerminalTabContents
              terminals={terminals}
              environmentId={environmentId}
              devProcessId={devProcessId}
              devOutput={devOutput}
              isStoppingDev={isStoppingDev}
            />
          </SessionTabs>
        </SessionPanel>
      </Panel>
    </Group>
  );
}

function useTaskRightPanel({
  sessionId,
  repositoryId,
  initialScripts = [],
  initialTerminals,
}: Required<Pick<TaskRightPanelProps, "sessionId" | "repositoryId">> &
  Pick<TaskRightPanelProps, "initialScripts" | "initialTerminals">) {
  const rightLayoutKey = "task-layout-right-v2";
  const { defaultLayout: rightLayout, onLayoutChanged: onRightLayoutChange } = useDefaultLayout({
    id: rightLayoutKey,
    panelIds: ["top", "bottom"],
    baseLayout: DEFAULT_RIGHT_LAYOUT,
  });

  const [isBottomCollapsed, setIsBottomCollapsed] = useState<boolean>(() =>
    getLocalStorage("task-right-panel-collapsed", false),
  );

  const setRightPanelActiveTab = useAppStore((state) => state.setRightPanelActiveTab);
  const environmentId = useAppStore((state) =>
    sessionId ? (state.environmentIdBySessionId[sessionId] ?? null) : null,
  );
  const closeLayoutPreview = useLayoutStore((state) => state.closePreview);

  // Use the terminals hook — env-keyed for shell ops, session-keyed for tab UX
  const terminalsApi = useTerminals({ sessionId, environmentId, initialTerminals });
  const {
    terminals,
    activeTab,
    handleCloseDevTab: baseHandleCloseDevTab,
    handleCloseTab,
    renameTerminal,
    destroyTerminal,
  } = terminalsApi;

  // Wrap handleCloseDevTab to also close the layout preview
  const handleCloseDevTab = useCallback(
    async (event: MouseEvent) => {
      await baseHandleCloseDevTab(event);
      if (sessionId) {
        closeLayoutPreview(sessionId);
      }
    },
    [baseHandleCloseDevTab, sessionId, closeLayoutPreview],
  );

  const { scripts, hasScripts } = useRightPanelScripts(repositoryId, initialScripts);
  useRightPanelPersistence({
    sessionId,
    hasScripts,
    activeTab,
    isBottomCollapsed,
    setRightPanelActiveTab,
  });
  const closeConfirm = useConfirmableTerminalClose({ terminals, handleCloseTab, destroyTerminal });
  const { tabs, handleTabChange } = useRightPanelTabs({
    hasScripts,
    terminals,
    handleCloseDevTab,
    handleCloseTab: closeConfirm.handleAskCloseTab,
    renameTerminal,
    requestCloseFromContext: closeConfirm.requestClose,
    sessionId,
    setRightPanelActiveTab,
  });

  return {
    ...terminalsApi,
    rightLayoutKey,
    rightLayout,
    onRightLayoutChange,
    isBottomCollapsed,
    setIsBottomCollapsed,
    environmentId,
    scripts,
    tabs,
    handleTabChange,
    closeConfirm,
  };
}

const TaskRightPanel = memo(function TaskRightPanel({
  topPanel,
  sessionId = null,
  repositoryId = null,
  initialScripts = [],
  initialTerminals,
}: TaskRightPanelProps) {
  const { t } = useTranslation();
  const {
    terminals,
    parkedTerminals,
    terminalTabValue,
    addTerminal,
    handleRunCommand,
    resumeTerminal,
    destroyTerminal,
    isStoppingDev,
    devProcessId,
    devOutput,
    rightLayoutKey,
    rightLayout,
    onRightLayoutChange,
    isBottomCollapsed,
    setIsBottomCollapsed,
    environmentId,
    scripts,
    tabs,
    handleTabChange,
    closeConfirm,
  } = useTaskRightPanel({ sessionId, repositoryId, initialScripts, initialTerminals });
  const { pendingClose, setPendingClose, closeAnchorRef, handleConfirmClose } = closeConfirm;
  return (
    <>
      <RightPanelContent
        isBottomCollapsed={isBottomCollapsed}
        topPanel={topPanel}
        tabs={tabs}
        terminalTabValue={terminalTabValue}
        handleTabChange={handleTabChange}
        addTerminal={addTerminal}
        setIsBottomCollapsed={setIsBottomCollapsed}
        rightLayoutKey={rightLayoutKey}
        rightLayout={rightLayout}
        onRightLayoutChange={onRightLayoutChange}
        scripts={scripts}
        handleRunCommand={handleRunCommand}
        terminals={terminals}
        parkedTerminals={parkedTerminals}
        resumeTerminal={resumeTerminal}
        destroyTerminal={destroyTerminal}
        environmentId={environmentId}
        devProcessId={devProcessId}
        devOutput={devOutput}
        isStoppingDev={isStoppingDev}
      />
      <CloseTerminalConfirmPopover
        open={pendingClose !== null}
        terminalName={pendingClose?.label || t("task:terminal")}
        anchorRef={closeAnchorRef}
        onOpenChange={(open) => {
          if (!open) setPendingClose(null);
        }}
        onConfirm={handleConfirmClose}
      />
    </>
  );
});

export { TaskRightPanel };
