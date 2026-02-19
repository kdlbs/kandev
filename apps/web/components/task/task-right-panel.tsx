"use client";

import type { ReactNode, MouseEvent } from "react";
import { memo, useEffect, useCallback, useState, useMemo } from "react";
import { Badge } from "@kandev/ui/badge";
import { SessionPanel, SessionPanelContent } from "@kandev/ui/pannel-session";
import { Group, Panel } from "react-resizable-panels";
import { TabsContent } from "@kandev/ui/tabs";
import {
  getLocalStorage,
  setLocalStorage,
  getSessionStorage,
  setSessionStorage,
} from "@/lib/local-storage";
import { ShellTerminal } from "@/components/task/shell-terminal";
import { PassthroughTerminal } from "@/components/task/passthrough-terminal";
import { useAppStore } from "@/components/state-provider";
import { useLayoutStore } from "@/lib/state/layout-store";
import { useDefaultLayout } from "@/lib/layout/use-default-layout";
import { SessionTabs, type SessionTab } from "@/components/session-tabs";
import { useRepositoryScripts } from "@/hooks/domains/workspace/use-repository-scripts";
import { useTerminals } from "@/hooks/domains/session/use-terminals";
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

function buildTerminalTabs(
  terminals: Terminal[],
  handleCloseDevTab: (event: MouseEvent) => void,
  handleCloseTab: (event: MouseEvent, id: string) => void,
): SessionTab[] {
  return terminals.map((terminal) => ({
    id: terminal.id,
    label: terminal.label,
    className: terminal.closable ? "group flex items-center cursor-pointer" : "cursor-pointer",
    closable: terminal.closable,
    onClose:
      terminal.type === "dev-server"
        ? handleCloseDevTab
        : (e: MouseEvent) => handleCloseTab(e, terminal.id),
  }));
}

function useRightPanelScripts(repositoryId: string | null, initialScripts: RepositoryScript[]) {
  const { scripts: storeScripts, isLoaded: scriptsLoaded } = useRepositoryScripts(repositoryId);
  const scripts = !scriptsLoaded && initialScripts.length > 0 ? initialScripts : storeScripts;
  return { scripts, hasScripts: scripts.length > 0 };
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

const TaskRightPanel = memo(function TaskRightPanel({
  topPanel,
  sessionId = null,
  repositoryId = null,
  initialScripts = [],
  initialTerminals,
}: TaskRightPanelProps) {
  const rightPanelIds = ["top", "bottom"];
  const rightLayoutKey = "task-layout-right-v2";
  const { defaultLayout: rightLayout, onLayoutChanged: onRightLayoutChange } = useDefaultLayout({
    id: rightLayoutKey,
    panelIds: rightPanelIds,
    baseLayout: DEFAULT_RIGHT_LAYOUT,
  });

  const [isBottomCollapsed, setIsBottomCollapsed] = useState<boolean>(() =>
    getLocalStorage("task-right-panel-collapsed", false),
  );

  const setRightPanelActiveTab = useAppStore((state) => state.setRightPanelActiveTab);
  const closeLayoutPreview = useLayoutStore((state) => state.closePreview);

  // Use the terminals hook
  const {
    terminals,
    activeTab,
    terminalTabValue,
    addTerminal,
    handleCloseDevTab: baseHandleCloseDevTab,
    handleCloseTab,
    handleRunCommand,
    isStoppingDev,
    devProcessId,
    devOutput,
  } = useTerminals({ sessionId, initialTerminals });

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

  // Restore 'commands' tab from sessionStorage (needs hasScripts check)
  useEffect(() => {
    if (!sessionId || !hasScripts) return;
    const savedTab = getSessionStorage<string | null>(`rightPanel-tab-${sessionId}`, null);
    if (savedTab === "commands") {
      setRightPanelActiveTab(sessionId, savedTab);
    }
  }, [sessionId, hasScripts, setRightPanelActiveTab]);

  // Persist active tab to sessionStorage when it changes
  useEffect(() => {
    if (!sessionId || !activeTab) return;
    setSessionStorage(`rightPanel-tab-${sessionId}`, activeTab);
  }, [sessionId, activeTab]);

  // Save collapse state to local storage
  useEffect(() => {
    setLocalStorage("task-right-panel-collapsed", isBottomCollapsed);
  }, [isBottomCollapsed]);

  const tabs: SessionTab[] = useMemo(() => {
    const commandsTabs: SessionTab[] = hasScripts ? [{ id: "commands", label: "Commands" }] : [];
    return [...commandsTabs, ...buildTerminalTabs(terminals, handleCloseDevTab, handleCloseTab)];
  }, [hasScripts, terminals, handleCloseDevTab, handleCloseTab]);

  const handleTabChange = useCallback(
    (value: string) => {
      if (sessionId) {
        setRightPanelActiveTab(sessionId, value);
      }
    },
    [sessionId, setRightPanelActiveTab],
  );

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
      <Panel id="top" minSize={30} className="min-h-0">
        {topPanel}
      </Panel>
      <Panel id="bottom" minSize={20} className="min-h-0">
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
            className="flex-1 min-h-0"
          >
            <CommandsTabContent scripts={scripts} onRunCommand={handleRunCommand} />
            <TerminalTabContents
              terminals={terminals}
              sessionId={sessionId}
              devProcessId={devProcessId}
              devOutput={devOutput}
              isStoppingDev={isStoppingDev}
            />
          </SessionTabs>
        </SessionPanel>
      </Panel>
    </Group>
  );
});

/** Commands tab content showing repository scripts */
function CommandsTabContent({
  scripts,
  onRunCommand,
}: {
  scripts: RepositoryScript[];
  onRunCommand: (script: RepositoryScript) => void;
}) {
  return (
    <TabsContent value="commands" className="flex-1 min-h-0">
      <SessionPanelContent>
        <div className="grid gap-2">
          {scripts.map((script) => (
            <button
              key={script.id}
              type="button"
              onClick={() => onRunCommand(script)}
              className="flex items-center gap-2 rounded-md border border-border px-3 py-2 text-sm text-left hover:bg-muted cursor-pointer min-w-0"
            >
              <span className="flex-1 min-w-0 truncate text-xs">{script.name}</span>
              <Badge variant="secondary" className="shrink-0 font-mono text-xs max-w-[60%] min-w-0">
                <span className="truncate block">{script.command}</span>
              </Badge>
            </button>
          ))}
        </div>
      </SessionPanelContent>
    </TabsContent>
  );
}

/** Terminal tab contents (dev-server and shell terminals) */
function TerminalTabContents({
  terminals,
  sessionId,
  devProcessId,
  devOutput,
  isStoppingDev,
}: {
  terminals: Terminal[];
  sessionId: string | null;
  devProcessId: string | null | undefined;
  devOutput: string | undefined;
  isStoppingDev: boolean;
}) {
  return (
    <>
      {terminals.map((terminal) => (
        <TabsContent key={terminal.id} value={terminal.id} className="flex-1 min-h-0">
          <SessionPanelContent className="p-0">
            {terminal.type === "dev-server" ? (
              <ShellTerminal
                key={devProcessId}
                processOutput={devOutput}
                processId={devProcessId ?? null}
                isStopping={isStoppingDev}
              />
            ) : (
              <PassthroughTerminal
                key={terminal.id}
                sessionId={sessionId ?? undefined}
                mode="shell"
                terminalId={terminal.id}
                label={terminal.type === "shell" ? terminal.label : undefined}
              />
            )}
          </SessionPanelContent>
        </TabsContent>
      ))}
    </>
  );
}

export { TaskRightPanel };
