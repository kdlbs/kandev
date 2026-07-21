"use client";

import type { ReactNode, MouseEvent } from "react";
import type { Layout } from "react-resizable-panels";
import { memo, useEffect, useCallback, useState, useMemo } from "react";
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
import { shouldConfirmTerminalClose } from "@/lib/terminal/terminal-busy-registry";
import { ParkedTerminalsMenu } from "@/components/task/parked-terminals-menu";
import { CloseTerminalConfirmDialog } from "@/components/task/close-terminal-confirm-dialog";
import {
  CommandsTabContent,
  TerminalTabContents,
} from "@/components/task/task-right-panel-tab-contents";
import type { RepositoryScript } from "@/lib/types/http";
import type { Terminal } from "@/hooks/domains/session/use-terminals";

type TaskRightPanelProps = {
  topPanel: ReactNode;
  sessionId?: string | null;
  repositoryId?: string | null;
  initialScripts?: RepositoryScript[];
  initialTerminals?: Terminal[];
};

const DEFAULT_RIGHT_LAYOUT: Record<string, number> = { top: 55, bottom: 45 };

function pickCurrentRenameValue(terminal: Terminal): string {
  if (terminal.customName && terminal.customName !== "") return terminal.customName;
  if (terminal.seq) return `Terminal ${terminal.seq}`;
  return terminal.label;
}

type TerminalTabsBuilderOpts = {
  terminals: Terminal[];
  handleCloseDevTab: (event: MouseEvent) => void;
  handleCloseTab: (event: MouseEvent, id: string) => void;
  onContextMenu?: (event: MouseEvent, terminal: Terminal) => void;
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
  onContextMenu,
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
      onContextMenu:
        isOrdinary && onContextMenu ? (e: MouseEvent) => onContextMenu(e, terminal) : undefined,
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

function useRightPanelTabs({
  hasScripts,
  terminals,
  handleCloseDevTab,
  handleCloseTab,
  renameTerminal,
  destroyTerminal,
  sessionId,
  setRightPanelActiveTab,
}: {
  hasScripts: boolean;
  terminals: Terminal[];
  handleCloseDevTab: (event: MouseEvent) => void;
  handleCloseTab: (event: MouseEvent, terminalId: string) => void;
  renameTerminal: (id: string, name: string | null) => Promise<void> | void;
  destroyTerminal: (id: string) => Promise<void> | void;
  sessionId: string | null;
  setRightPanelActiveTab: (sessionId: string, tabId: string) => void;
}) {
  const onContextMenu = useCallback(
    (event: MouseEvent, terminal: Terminal) => {
      event.preventDefault();
      // Browser prompt() is intentionally low-tech; the next iteration
      // promotes this to a real shadcn ContextMenu, but window.prompt
      // ships the renameable-terminal requirement today.
      const current = pickCurrentRenameValue(terminal);
      const choice = window.prompt(
        `Rename terminal (leave empty to reset, type "destroy" to remove and stop the PTY).`,
        current,
      );
      if (choice === null) return;
      const trimmed = choice.trim();
      if (trimmed.toLowerCase() === "destroy") {
        void destroyTerminal(terminal.id);
        return;
      }
      void renameTerminal(terminal.id, trimmed === "" ? null : trimmed);
    },
    [renameTerminal, destroyTerminal],
  );

  const tabs: SessionTab[] = useMemo(() => {
    const commandsTabs: SessionTab[] = hasScripts ? [{ id: "commands", label: "Commands" }] : [];
    return [
      ...commandsTabs,
      ...buildTerminalTabs({
        terminals,
        handleCloseDevTab,
        handleCloseTab,
        onContextMenu,
        onDoubleClick: onContextMenu,
      }),
    ];
  }, [hasScripts, terminals, handleCloseDevTab, handleCloseTab, onContextMenu]);

  const handleTabChange = useCallback(
    (value: string) => {
      if (sessionId) setRightPanelActiveTab(sessionId, value);
    },
    [sessionId, setRightPanelActiveTab],
  );

  return { tabs, handleTabChange };
}

/**
 * Gate the X-button close behind a confirm when the terminal looks busy or is
 * a script terminal — mirrors the dockview tab and mobile picker so every
 * close path warns before it destroys a running shell. Idle shells still close
 * immediately via the underlying `handleCloseTab`.
 */
function useConfirmableTerminalClose({
  terminals,
  handleCloseTab,
  destroyTerminal,
}: {
  terminals: Terminal[];
  handleCloseTab: (event: MouseEvent, terminalId: string) => void;
  destroyTerminal: (id: string) => Promise<void>;
}) {
  const [pendingClose, setPendingClose] = useState<Terminal | null>(null);

  const handleAskCloseTab = useCallback(
    (event: MouseEvent, terminalId: string) => {
      const terminal = terminals.find((t) => t.id === terminalId);
      const needsConfirm =
        !!terminal &&
        shouldConfirmTerminalClose(terminalId, {
          type: terminal.type,
          kind: terminal.kind,
        });
      if (needsConfirm) {
        event.preventDefault();
        event.stopPropagation();
        setPendingClose(terminal);
        return;
      }
      handleCloseTab(event, terminalId);
    },
    [terminals, handleCloseTab],
  );

  const handleConfirmClose = useCallback(async () => {
    if (!pendingClose) return;
    await destroyTerminal(pendingClose.id);
    setPendingClose(null);
  }, [pendingClose, destroyTerminal]);

  return { pendingClose, setPendingClose, handleAskCloseTab, handleConfirmClose };
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
  destroyTerminal: (id: string) => Promise<void> | void;
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
                  onDestroy={destroyTerminal}
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
    destroyTerminal,
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
  const { pendingClose, setPendingClose, handleConfirmClose } = closeConfirm;
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
      <CloseTerminalConfirmDialog
        open={pendingClose !== null}
        terminalName={pendingClose?.label || "Terminal"}
        onOpenChange={(open) => {
          if (!open) setPendingClose(null);
        }}
        onConfirm={handleConfirmClose}
      />
    </>
  );
});

export { TaskRightPanel };
