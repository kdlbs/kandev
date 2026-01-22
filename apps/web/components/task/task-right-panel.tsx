'use client';

import type { ReactNode, MouseEvent } from 'react';
import { memo, useEffect, useState, useCallback } from 'react';
import { IconChevronDown, IconChevronUp, IconX } from '@tabler/icons-react';
import { Badge } from '@kandev/ui/badge';
import { SessionPanel, SessionPanelContent } from '@kandev/ui/pannel-session';
import { ResizableHandle, ResizablePanel, ResizablePanelGroup } from '@kandev/ui/resizable';
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@kandev/ui/tabs';
import { getLocalStorage, setLocalStorage } from '@/lib/local-storage';
import { COMMANDS } from '@/components/task/task-data';
import { ShellTerminal } from '@/components/task/shell-terminal';
import { useAppStore } from '@/components/state-provider';
import { useLayoutStore } from '@/lib/state/layout-store';
import { stopProcess } from '@/lib/api';
import type { Layout } from 'react-resizable-panels';

type TerminalType = 'commands' | 'dev-server' | 'shell';

type Terminal = {
  id: string;
  type: TerminalType;
  label: string;
};

type TaskRightPanelProps = {
  topPanel: ReactNode;
  sessionId?: string | null;
};

const DEFAULT_RIGHT_LAYOUT: Layout = { top: 55, bottom: 45 };

const TaskRightPanel = memo(function TaskRightPanel({ topPanel, sessionId = null }: TaskRightPanelProps) {
  const [rightLayout, setRightLayout] = useState<Layout>(() =>
    getLocalStorage('task-layout-right', DEFAULT_RIGHT_LAYOUT)
  );
  const [terminals, setTerminals] = useState<Terminal[]>([
    { id: 'shell-1', type: 'shell', label: 'Terminal' },
  ]);
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

  // Sync dev server terminal with preview state
  useEffect(() => {
    if (!sessionId) return;

    setTerminals((prev) => {
      const hasDevTerminal = prev.some((t) => t.type === 'dev-server');

      if (previewOpen && !hasDevTerminal) {
        // Add dev server terminal at the beginning
        return [{ id: 'dev-server', type: 'dev-server', label: 'Dev Server' }, ...prev];
      }

      if (!previewOpen && hasDevTerminal) {
        // Remove dev server terminal
        return prev.filter((t) => t.type !== 'dev-server');
      }

      return prev;
    });
  }, [previewOpen, sessionId]);

  const addTerminal = useCallback(() => {
    setTerminals((prev) => {
      const shellTerminals = prev.filter((t) => t.type === 'shell');
      const nextNum = shellTerminals.length + 1;
      const newTerminal: Terminal = {
        id: `shell-${nextNum}`,
        type: 'shell',
        label: nextNum === 1 ? 'Terminal' : `Terminal ${nextNum}`,
      };
      if (sessionId) {
        setRightPanelActiveTab(sessionId, newTerminal.id);
      }
      return [...prev, newTerminal];
    });
  }, [sessionId, setRightPanelActiveTab]);

  const removeTerminal = useCallback(
    (id: string) => {
      setTerminals((prev) => {
        const shellTerminals = prev.filter((t) => t.type === 'shell');
        if (shellTerminals.length <= 1) {
          return prev; // Keep at least one shell terminal
        }

        const nextTerminals = prev.filter((t) => t.id !== id);

        // If removing active tab, switch to first available shell terminal
        if (activeTab === id) {
          const fallbackShell = nextTerminals.find((t) => t.type === 'shell');
          if (fallbackShell && sessionId) {
            setRightPanelActiveTab(sessionId, fallbackShell.id);
          }
        }

        return nextTerminals;
      });
    },
    [activeTab, sessionId, setRightPanelActiveTab]
  );

  const terminalTabValue = activeTab ?? terminals.find((t) => t.type === 'shell')?.id ?? 'commands';

  // Validate active tab exists in terminals
  useEffect(() => {
    if (!sessionId || !activeTab) return;

    const tabExists =
      activeTab === 'commands' ||
      terminals.some((t) => t.id === activeTab);

    if (!tabExists) {
      const fallbackShell = terminals.find((t) => t.type === 'shell');
      if (fallbackShell) {
        setRightPanelActiveTab(sessionId, fallbackShell.id);
      }
    }
  }, [activeTab, sessionId, terminals, setRightPanelActiveTab]);

  const handleCloseDevTab = useCallback(
    async (event: MouseEvent) => {
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
      const fallbackShell = terminals.find((t) => t.type === 'shell');
      if (fallbackShell) {
        setRightPanelActiveTab(sessionId, fallbackShell.id);
      }
      setPreviewOpen(sessionId, false);
      setPreviewStage(sessionId, 'closed');
      closeLayoutPreview(sessionId);
    },
    [sessionId, devProcessId, terminals, setRightPanelActiveTab, setPreviewOpen, setPreviewStage, closeLayoutPreview]
  );

  if (isBottomCollapsed) {
    return (
      <div className="h-full min-h-0 flex flex-col gap-1">
        <div className="flex-1 min-h-0">{topPanel}</div>
        <SessionPanel
          borderSide="left"
          className="!h-12 !p-0 px-3 mt-[2px] justify-between items-center flex-row"
        >
          <Tabs
            value={terminalTabValue}
            onValueChange={(value) => {
              if (sessionId) {
                setRightPanelActiveTab(sessionId, value);
              }
            }}
            className="flex-1 min-h-0"
          >
            <TabsList>
              <TabsTrigger value="commands" className="cursor-pointer">
                Commands
              </TabsTrigger>
              {terminals.map((terminal) => (
                <TabsTrigger
                  key={terminal.id}
                  value={terminal.id}
                  className={
                    terminal.type === 'dev-server'
                      ? 'group flex items-center gap-1 pr-1 cursor-pointer'
                      : 'cursor-pointer'
                  }
                >
                  {terminal.label}
                  {terminal.type === 'dev-server' && (
                    <span
                      role="button"
                      tabIndex={-1}
                      className="text-muted-foreground opacity-0 group-hover:opacity-100 hover:text-foreground"
                      onClick={handleCloseDevTab}
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
          </Tabs>
          <button
            type="button"
            className="text-muted-foreground hover:text-foreground cursor-pointer"
            onClick={() => setIsBottomCollapsed(false)}
          >
            <IconChevronUp className="h-4 w-4" />
          </button>
        </SessionPanel>
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
        <SessionPanel borderSide="left" margin="top">
          <Tabs
            value={terminalTabValue}
            onValueChange={(value) => {
              if (sessionId) {
                setRightPanelActiveTab(sessionId, value);
              }
            }}
            className="flex-1 min-h-0"
          >
            <div className="flex items-center justify-between mb-3">
              <TabsList>
                <TabsTrigger value="commands" className="cursor-pointer">
                  Commands
                </TabsTrigger>
                {terminals.map((terminal) => (
                  <TabsTrigger
                    key={terminal.id}
                    value={terminal.id}
                    className="group flex items-center gap-1 pr-1 cursor-pointer"
                  >
                    {terminal.label}
                    {terminal.type === 'dev-server' ? (
                      <span
                        role="button"
                        tabIndex={-1}
                        className="text-muted-foreground opacity-0 group-hover:opacity-100 hover:text-foreground"
                        onClick={handleCloseDevTab}
                      >
                        <IconX className="h-3.5 w-3.5" />
                      </span>
                    ) : terminal.type === 'shell' && terminals.filter((t) => t.type === 'shell').length > 1 ? (
                      <span
                        role="button"
                        tabIndex={-1}
                        className="text-muted-foreground opacity-0 group-hover:opacity-100 hover:text-foreground"
                        onClick={(event) => {
                          event.preventDefault();
                          event.stopPropagation();
                          removeTerminal(terminal.id);
                        }}
                      >
                        <IconX className="h-3.5 w-3.5" />
                      </span>
                    ) : null}
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
              <SessionPanelContent>
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
              </SessionPanelContent>
            </TabsContent>
            {terminals.map((terminal) => (
              <TabsContent key={terminal.id} value={terminal.id} className="flex-1 min-h-0">
                <SessionPanelContent className="p-0">
                  {terminal.type === 'dev-server' ? (
                    <ShellTerminal
                      processOutput={devOutput}
                      processId={devProcessId ?? null}
                      isStopping={isStoppingDev}
                    />
                  ) : (
                    <ShellTerminal sessionId={sessionId ?? undefined} />
                  )}
                </SessionPanelContent>
              </TabsContent>
            ))}
          </Tabs>
        </SessionPanel>
      </ResizablePanel>
    </ResizablePanelGroup>
  );
});

export { TaskRightPanel };
