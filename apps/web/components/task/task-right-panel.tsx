'use client';

import type { ReactNode, MouseEvent } from 'react';
import { memo, useEffect, useState } from 'react';
import { IconChevronDown, IconChevronUp, IconX } from '@tabler/icons-react';
import { Badge } from '@kandev/ui/badge';
import { ResizableHandle, ResizablePanel, ResizablePanelGroup } from '@kandev/ui/resizable';
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@kandev/ui/tabs';
import { getLocalStorage, setLocalStorage } from '@/lib/local-storage';
import { COMMANDS } from '@/components/task/task-data';
import { ShellTerminal } from '@/components/task/shell-terminal';
import { ProcessOutputTerminal } from '@/components/task/process-output-terminal';
import { useAppStore } from '@/components/state-provider';
import { useLayoutStore } from '@/lib/state/layout-store';
import { stopProcess } from '@/lib/api';
import type { Layout } from 'react-resizable-panels';

type TaskRightPanelProps = {
  topPanel: ReactNode;
  sessionId?: string | null;
};

const DEFAULT_RIGHT_LAYOUT: Layout = { top: 55, bottom: 45 };

const TaskRightPanel = memo(function TaskRightPanel({ topPanel, sessionId = null }: TaskRightPanelProps) {
  const [rightLayout, setRightLayout] = useState<Layout>(() =>
    getLocalStorage('task-layout-right', DEFAULT_RIGHT_LAYOUT)
  );
  const [activeTerminalId, setActiveTerminalId] = useState(1);
  const [terminalIds, setTerminalIds] = useState([1]);
  const [isBottomCollapsed, setIsBottomCollapsed] = useState(false);
  const [isStoppingDev, setIsStoppingDev] = useState(false);
  const activeTab = useAppStore((state) =>
    sessionId ? state.rightPanel.activeTabBySessionId[sessionId] : undefined
  );
  const setRightPanelActiveTab = useAppStore((state) => state.setRightPanelActiveTab);
  const closeLayoutPreview = useLayoutStore((state) => state.closePreview);
  const setPreviewOpen = useAppStore((state) => state.setPreviewOpen);
  const setPreviewStage = useAppStore((state) => state.setPreviewStage);
  const devProcessId = useAppStore((state) =>
    sessionId ? state.processes.devProcessBySessionId[sessionId] : undefined
  );
  const devOutput = useAppStore((state) =>
    devProcessId ? state.processes.outputsByProcessId[devProcessId] ?? '' : ''
  );
  const previewOpen = useAppStore((state) =>
    sessionId ? state.previewPanel.openBySessionId[sessionId] ?? false : false
  );
  const showDevTab = Boolean(sessionId && previewOpen);

  const addTerminal = () => {
    setTerminalIds((ids) => {
      const nextId = Math.max(0, ...ids) + 1;
      setActiveTerminalId(nextId);
      if (sessionId) {
        setRightPanelActiveTab(sessionId, `terminal-${nextId}`);
      }
      return [...ids, nextId];
    });
  };

  const removeTerminal = (id: number) => {
    setTerminalIds((ids) => {
      const nextIds = ids.filter((terminalId) => terminalId !== id);
      if (activeTerminalId === id) {
        const fallback = nextIds[0] ?? 1;
        setActiveTerminalId(fallback);
        if (sessionId) {
          setRightPanelActiveTab(sessionId, `terminal-${fallback}`);
        }
      }
      return nextIds.length ? nextIds : [1];
    });
  };

  const terminalTabValue =
    activeTab ?? (activeTerminalId === 0 ? 'commands' : `terminal-${activeTerminalId}`);

  useEffect(() => {
    if (!sessionId) return;
    if (activeTab && activeTab.startsWith('terminal-')) {
      const parsed = Number(activeTab.replace('terminal-', ''));
      if (!Number.isNaN(parsed) && parsed !== activeTerminalId) {
        setActiveTerminalId(parsed);
      }
    }
  }, [activeTab, activeTerminalId, sessionId]);

  useEffect(() => {
    if (!sessionId || !activeTab) return;
    if (activeTab.startsWith('terminal-')) {
      const parsed = Number(activeTab.replace('terminal-', ''));
      if (Number.isNaN(parsed) || !terminalIds.includes(parsed)) {
        const fallback = terminalIds[0] ?? 1;
        setActiveTerminalId(fallback);
        setRightPanelActiveTab(sessionId, `terminal-${fallback}`);
      }
    }
  }, [activeTab, sessionId, terminalIds, setRightPanelActiveTab]);

  useEffect(() => {
    if (!sessionId) return;
    if (!showDevTab && terminalTabValue === 'dev-server') {
      const fallback = `terminal-${terminalIds[0] ?? 1}`;
      setRightPanelActiveTab(sessionId, fallback);
    }
  }, [showDevTab, sessionId, terminalTabValue, terminalIds, setRightPanelActiveTab]);

  const handleCloseDevTab = async (event: MouseEvent) => {
    event.preventDefault();
    event.stopPropagation();
    if (!sessionId) return;
    if (devProcessId) {
      setIsStoppingDev(true);
      try {
        await stopProcess(sessionId, { process_id: devProcessId });
      } finally {
        setIsStoppingDev(false);
      }
    }
    setRightPanelActiveTab(sessionId, `terminal-${terminalIds[0] ?? 1}`);
    setPreviewOpen(sessionId, false);
    setPreviewStage(sessionId, 'closed');
    closeLayoutPreview(sessionId);
  };

  if (isBottomCollapsed) {
    return (
      <div className="h-full min-h-0 flex flex-col gap-1">
        <div className="flex-1 min-h-0">{topPanel}</div>
        <div className="h-12 border border-border/70 rounded-lg bg-card flex items-center justify-between px-3 border-l-0 mt-[2px]">
          <Tabs
            value={terminalTabValue}
            onValueChange={(value) => {
              if (value === 'commands') {
                setActiveTerminalId(0);
                if (sessionId) setRightPanelActiveTab(sessionId, value);
                return;
              }
              if (value === 'dev-server') {
                if (sessionId) setRightPanelActiveTab(sessionId, value);
                return;
              }
              const parsed = Number(value.replace('terminal-', ''));
              if (!Number.isNaN(parsed)) {
                setActiveTerminalId(parsed);
                if (sessionId) setRightPanelActiveTab(sessionId, value);
              }
            }}
            className="flex-1 min-h-0"
          >
            <TabsList>
              <TabsTrigger value="commands" className="cursor-pointer">
                Commands
              </TabsTrigger>
              {showDevTab ? (
                <TabsTrigger value="dev-server" className="group flex items-center gap-1 pr-1 cursor-pointer">
                  Dev Server
                  <span
                    role="button"
                    tabIndex={-1}
                    className="text-muted-foreground opacity-0 group-hover:opacity-100 hover:text-foreground"
                    onClick={handleCloseDevTab}
                  >
                    <IconX className="h-3.5 w-3.5" />
                  </span>
                </TabsTrigger>
              ) : null}
              {terminalIds.map((id) => (
                <TabsTrigger key={id} value={`terminal-${id}`} className="cursor-pointer">
                  {id === 1 ? 'Terminal' : `Terminal ${id}`}
                </TabsTrigger>
              ))}
              <TabsTrigger value="add" onClick={addTerminal} className="cursor-pointer">
                +
              </TabsTrigger>
            </TabsList>
          </Tabs>
          <button
            type="button"
            className="text-muted-foreground hover:text-foreground cursor-pointer"
            onClick={() => setIsBottomCollapsed(false)}
          >
            <IconChevronUp className="h-4 w-4" />
          </button>
        </div>
      </div>
    );
  }

  return (
    <ResizablePanelGroup
      orientation="vertical"
      className="h-full"
      id="task-right-panel"
      defaultLayout={rightLayout}
      onLayoutChanged={(sizes) => {
        setRightLayout(sizes);
        setLocalStorage('task-layout-right', sizes);
      }}
    >
      <ResizablePanel id="top" defaultSize={rightLayout.top} minSize={30}>
        {topPanel}
      </ResizablePanel>
      <ResizableHandle className="h-px" />
      <ResizablePanel id="bottom" defaultSize={rightLayout.bottom} minSize={20}>
        <div className="h-full min-h-0 bg-card p-4 flex flex-col rounded-lg border border-border/70 border-l-0 mt-[5px]">
          <Tabs
            value={terminalTabValue}
            onValueChange={(value) => {
              if (value === 'commands') {
                setActiveTerminalId(0);
                if (sessionId) setRightPanelActiveTab(sessionId, value);
                return;
              }
              if (value === 'dev-server') {
                if (sessionId) setRightPanelActiveTab(sessionId, value);
                return;
              }
              const parsed = Number(value.replace('terminal-', ''));
              if (!Number.isNaN(parsed)) {
                setActiveTerminalId(parsed);
                if (sessionId) setRightPanelActiveTab(sessionId, value);
              }
            }}
            className="flex-1 min-h-0"
          >
            <div className="flex items-center justify-between mb-3">
              <TabsList>
                <TabsTrigger value="commands" className="cursor-pointer">
                  Commands
                </TabsTrigger>
                {showDevTab ? (
                  <TabsTrigger value="dev-server" className="group flex items-center gap-1 pr-1 cursor-pointer">
                    Dev Server
                    <span
                      role="button"
                      tabIndex={-1}
                      className="text-muted-foreground opacity-0 group-hover:opacity-100 hover:text-foreground"
                      onClick={handleCloseDevTab}
                    >
                      <IconX className="h-3.5 w-3.5" />
                    </span>
                  </TabsTrigger>
                ) : null}
                {terminalIds.map((id) => (
                  <TabsTrigger
                    key={id}
                    value={`terminal-${id}`}
                    className="group flex items-center gap-1 pr-1 cursor-pointer"
                  >
                    {id === 1 ? 'Terminal' : `Terminal ${id}`}
                    {terminalIds.length > 1 && (
                      <span
                        role="button"
                        tabIndex={-1}
                        className="text-muted-foreground opacity-0 group-hover:opacity-100 hover:text-foreground"
                        onClick={(event) => {
                          event.preventDefault();
                          event.stopPropagation();
                          removeTerminal(id);
                        }}
                      >
                        <IconX className="h-3.5 w-3.5" />
                      </span>
                    )}
                  </TabsTrigger>
                ))}
                <TabsTrigger value="add" onClick={addTerminal} className="cursor-pointer">
                  +
                </TabsTrigger>
              </TabsList>
              <button
                type="button"
                className="text-muted-foreground hover:text-foreground cursor-pointer"
                onClick={() => setIsBottomCollapsed(true)}
              >
                <IconChevronDown className="h-4 w-4" />
              </button>
            </div>
            <TabsContent value="commands" className="flex-1 min-h-0">
              <div className="flex-1 min-h-0 overflow-y-auto rounded-lg bg-background p-3 space-y-3 h-full">
                <div className="grid gap-2">
                  {COMMANDS.map((command) => (
                    <button
                      key={command.id}
                      type="button"
                      className="flex items-center justify-between rounded-md border border-border px-3 py-2 text-sm text-left hover:bg-muted"
                    >
                      <span>{command.label}</span>
                      <Badge variant="secondary">Run</Badge>
                    </button>
                  ))}
                </div>
              </div>
            </TabsContent>
            {showDevTab ? (
              <TabsContent value="dev-server" className="flex-1 min-h-0">
                <div className="flex-1 min-h-0 h-full p-1">
                  <ProcessOutputTerminal
                    output={devOutput}
                    processId={devProcessId ?? null}
                    isStopping={isStoppingDev}
                  />
                </div>
              </TabsContent>
            ) : null}
            {terminalIds.map((id) => (
              <TabsContent key={id} value={`terminal-${id}`} className="flex-1 min-h-0">
                <div className="flex-1 min-h-0 h-full p-1">
                  <ShellTerminal />
                </div>
              </TabsContent>
            ))}
          </Tabs>
        </div>
      </ResizablePanel>
    </ResizablePanelGroup>
  );
});

export { TaskRightPanel };
