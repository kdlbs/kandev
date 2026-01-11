'use client';

import type { ReactNode } from 'react';
import { memo, useState } from 'react';
import { IconChevronDown, IconChevronUp, IconX } from '@tabler/icons-react';
import { Badge } from '@/components/ui/badge';
import { ResizableHandle, ResizablePanel, ResizablePanelGroup } from '@/components/ui/resizable';
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs';
import { getLocalStorage, setLocalStorage } from '@/lib/local-storage';
import { COMMANDS } from '@/components/task/task-data';

type TaskRightPanelProps = {
  topPanel: ReactNode;
};

const TaskRightPanel = memo(function TaskRightPanel({ topPanel }: TaskRightPanelProps) {
  const defaultRightLayout: [number, number] = [55, 45];
  const [rightLayout, setRightLayout] = useState<[number, number]>(() =>
    getLocalStorage('task-layout-right', defaultRightLayout)
  );
  const [activeTerminalId, setActiveTerminalId] = useState(1);
  const [terminalIds, setTerminalIds] = useState([1]);
  const [isBottomCollapsed, setIsBottomCollapsed] = useState(false);

  const addTerminal = () => {
    setTerminalIds((ids) => {
      const nextId = Math.max(0, ...ids) + 1;
      setActiveTerminalId(nextId);
      return [...ids, nextId];
    });
  };

  const removeTerminal = (id: number) => {
    setTerminalIds((ids) => {
      const nextIds = ids.filter((terminalId) => terminalId !== id);
      if (activeTerminalId === id) {
        const fallback = nextIds[0] ?? 1;
        setActiveTerminalId(fallback);
      }
      return nextIds.length ? nextIds : [1];
    });
  };

  const terminalTabValue = activeTerminalId === 0 ? 'commands' : `terminal-${activeTerminalId}`;

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
                return;
              }
              const parsed = Number(value.replace('terminal-', ''));
              if (!Number.isNaN(parsed)) {
                setActiveTerminalId(parsed);
              }
            }}
            className="flex-1 min-h-0"
          >
            <TabsList>
              <TabsTrigger value="commands" className="cursor-pointer">
                Commands
              </TabsTrigger>
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
      direction="vertical"
      className="h-full"
      onLayout={(sizes) => {
        setRightLayout(sizes as [number, number]);
        setLocalStorage('task-layout-right', sizes);
      }}
    >
      <ResizablePanel defaultSize={rightLayout[0]} minSize={30}>
        {topPanel}
      </ResizablePanel>
      <ResizableHandle className="h-px" />
      <ResizablePanel defaultSize={rightLayout[1]} minSize={20}>
        <div className="h-full min-h-0 bg-card p-4 flex flex-col rounded-lg border border-border/70 border-l-0 mt-[5px]">
          <Tabs
            value={terminalTabValue}
            onValueChange={(value) => {
              if (value === 'commands') {
                setActiveTerminalId(0);
                return;
              }
              const parsed = Number(value.replace('terminal-', ''));
              if (!Number.isNaN(parsed)) {
                setActiveTerminalId(parsed);
              }
            }}
            className="flex-1 min-h-0"
          >
            <div className="flex items-center justify-between mb-3">
              <TabsList>
                <TabsTrigger value="commands" className="cursor-pointer">
                  Commands
                </TabsTrigger>
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
            {terminalIds.map((id) => (
              <TabsContent key={id} value={`terminal-${id}`} className="flex-1 min-h-0">
                <div className="flex-1 min-h-0 overflow-y-auto rounded-lg bg-background p-3 space-y-3 h-full">
                  <div className="rounded-md border border-border bg-black/90 text-green-200 font-mono text-xs p-3 space-y-2">
                    <p className="text-green-400">kan-dev@workspace:~$</p>
                    <p className="text-green-200">_</p>
                  </div>
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
