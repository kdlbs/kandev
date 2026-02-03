'use client';

import type { ReactNode, MouseEvent } from 'react';
import { memo, useEffect, useState, useCallback, useMemo, useRef } from 'react';
import { Badge } from '@kandev/ui/badge';
import { SessionPanel, SessionPanelContent } from '@kandev/ui/pannel-session';
import { Group, Panel } from 'react-resizable-panels';
import { TabsContent } from '@kandev/ui/tabs';
import { getLocalStorage, setLocalStorage, getSessionStorage, setSessionStorage } from '@/lib/local-storage';
import { ShellTerminal } from '@/components/task/shell-terminal';
import { useAppStore, useAppStoreApi } from '@/components/state-provider';
import { useLayoutStore } from '@/lib/state/layout-store';
import { stopProcess, startProcess, getSessionProcess } from '@/lib/api';
import { useDefaultLayout } from '@/lib/layout/use-default-layout';
import { SessionTabs, type SessionTab } from '@/components/session-tabs';
import { useRepositoryScripts } from '@/hooks/domains/workspace/use-repository-scripts';
import type { RepositoryScript } from '@/lib/types/http';

type TerminalType = 'commands' | 'dev-server' | 'shell' | 'custom';

type Terminal = {
  id: string;
  type: TerminalType;
  label: string;
  processId?: string;
};

type TaskRightPanelProps = {
  topPanel: ReactNode;
  sessionId?: string | null;
  repositoryId?: string | null;
  initialScripts?: RepositoryScript[];
};

const DEFAULT_RIGHT_LAYOUT: Record<string, number> = { top: 55, bottom: 45 };

const TaskRightPanel = memo(function TaskRightPanel({ topPanel, sessionId = null, repositoryId = null, initialScripts = [] }: TaskRightPanelProps) {
  const storeApi = useAppStoreApi();
  const rightPanelIds = ['top', 'bottom'];
  const rightLayoutKey = 'task-layout-right-v2';
  const { defaultLayout: rightLayout, onLayoutChanged: onRightLayoutChange } = useDefaultLayout({
    id: rightLayoutKey,
    panelIds: rightPanelIds,
    baseLayout: DEFAULT_RIGHT_LAYOUT,
  });
  // Initialize terminals from sessionStorage for persistence across remounts (within browser tab)
  const [terminals, setTerminals] = useState<Terminal[]>(() => {
    const baseTerminals: Terminal[] = [{ id: 'shell-1', type: 'shell', label: 'Terminal' }];
    if (!sessionId) return baseTerminals;
    const savedCustomTerminals = getSessionStorage<Terminal[]>(`customTerminals-${sessionId}`, []);
    if (savedCustomTerminals.length > 0) {
      return [...baseTerminals, ...savedCustomTerminals];
    }
    return baseTerminals;
  });
  const [isBottomCollapsed, setIsBottomCollapsed] = useState<boolean>(() =>
    getLocalStorage('task-right-panel-collapsed', false)
  );
  const [isStoppingDev, setIsStoppingDev] = useState(false);
  const activeTab = useAppStore((state) =>
    sessionId ? state.rightPanel.activeTabBySessionId[sessionId] : undefined
  );
  const setRightPanelActiveTab = useAppStore((state) => state.setRightPanelActiveTab);

  // Track if we've restored the tab from localStorage
  const tabRestoredRef = useRef(false);
  // Track the previous sessionId to detect changes
  const prevSessionIdRef = useRef(sessionId);
  // Track which process outputs we've already loaded (to prevent duplicates)
  const loadedOutputsRef = useRef<Set<string>>(new Set());
  // Track which process IDs have been explicitly closed (so sync effect doesn't re-add them)
  const closedProcessIdsRef = useRef<Set<string>>(new Set());

  // Track if we've initialized closedProcessIdsRef from sessionStorage
  const closedIdsInitializedRef = useRef(false);

  // Reset terminals and refs when sessionId changes
  useEffect(() => {
    if (prevSessionIdRef.current === sessionId) return;
    prevSessionIdRef.current = sessionId;

    // Reset refs
    tabRestoredRef.current = false;
    loadedOutputsRef.current = new Set();
    closedProcessIdsRef.current = new Set();
    closedIdsInitializedRef.current = false;

    // Reset terminals for new session
    const baseTerminals: Terminal[] = [{ id: 'shell-1', type: 'shell', label: 'Terminal' }];
    if (!sessionId) {
      setTerminals(baseTerminals);
      return;
    }
    const savedCustomTerminals = getSessionStorage<Terminal[]>(`customTerminals-${sessionId}`, []);
    if (savedCustomTerminals.length > 0) {
      setTerminals([...baseTerminals, ...savedCustomTerminals]);
    } else {
      setTerminals(baseTerminals);
    }
  }, [sessionId]);

  // Persist active tab to localStorage when it changes
  useEffect(() => {
    if (!sessionId || !activeTab) return;
    setLocalStorage(`rightPanel-tab-${sessionId}`, activeTab);
  }, [sessionId, activeTab]);

  // Persist custom terminals to sessionStorage when they change
  // Skip persisting during session transitions to avoid overwriting the new session's data
  useEffect(() => {
    if (!sessionId) return;
    // Only persist if we're not in a session transition (prevSessionIdRef is updated in reset effect)
    if (prevSessionIdRef.current !== sessionId) return;
    const customTerminals = terminals.filter(t => t.type === 'custom');
    setSessionStorage(`customTerminals-${sessionId}`, customTerminals);
  }, [sessionId, terminals]);

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

  // Repository scripts - use initialScripts as fallback for SSR
  const { scripts: storeScripts, isLoaded: scriptsLoaded } = useRepositoryScripts(repositoryId);
  const scripts = scriptsLoaded ? storeScripts : (initialScripts.length > 0 ? initialScripts : storeScripts);
  const hasScripts = scripts.length > 0;

  // Restore non-custom tabs from localStorage
  useEffect(() => {
    if (!sessionId || tabRestoredRef.current) return;

    const savedTab = getLocalStorage<string | null>(`rightPanel-tab-${sessionId}`, null);
    if (!savedTab) return;

    // Skip custom tabs - they're handled in a separate effect after terminals are set up
    if (savedTab.startsWith('custom-')) return;

    // For 'commands' tab, verify scripts exist (either from SSR or loaded)
    if (savedTab === 'commands') {
      if (hasScripts) {
        setRightPanelActiveTab(sessionId, savedTab);
        tabRestoredRef.current = true;
      }
      return;
    }

    // For shell tabs, restore immediately
    if (savedTab.startsWith('shell-')) {
      setRightPanelActiveTab(sessionId, savedTab);
      tabRestoredRef.current = true;
    }
  }, [sessionId, hasScripts, setRightPanelActiveTab]);

  // Process state for custom processes
  const processState = useAppStore((state) => state.processes);
  const appendProcessOutput = useAppStore((state) => state.appendProcessOutput);
  const clearProcessOutput = useAppStore((state) => state.clearProcessOutput);
  const upsertProcessStatus = useAppStore((state) => state.upsertProcessStatus);

  // Get custom processes for this session from store
  const customProcesses = useMemo(() => {
    if (!sessionId) return [];
    const processIds = processState.processIdsBySessionId[sessionId] ?? [];
    return processIds
      .map(id => processState.processesById[id])
      .filter(p => p && p.kind === 'custom');
  }, [sessionId, processState.processIdsBySessionId, processState.processesById]);

  // Initialize closedProcessIdsRef from sessionStorage on mount
  // This prevents re-adding terminals that were closed before page refresh
  useEffect(() => {
    if (!sessionId || customProcesses.length === 0) return;
    if (closedIdsInitializedRef.current) return;
    closedIdsInitializedRef.current = true;

    const savedCustomTerminals = getSessionStorage<Terminal[]>(`customTerminals-${sessionId}`, []);
    const savedProcessIds = new Set(
      savedCustomTerminals.map(t => t.processId).filter(Boolean)
    );

    // Any custom process in store that's not in savedTerminals was closed before refresh
    customProcesses.forEach(proc => {
      if (!savedProcessIds.has(proc.processId)) {
        closedProcessIdsRef.current.add(proc.processId);
      }
    });
  }, [sessionId, customProcesses]);

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

  // Sync custom terminals from store (adds any new processes not already in terminals)
  useEffect(() => {
    if (!sessionId || customProcesses.length === 0) return;

    setTerminals(prev => {
      const existingCustomIds = new Set(
        prev.filter(t => t.type === 'custom').map(t => t.processId)
      );

      const newTerminals = customProcesses
        .filter(p => !existingCustomIds.has(p.processId) && !closedProcessIdsRef.current.has(p.processId))
        .map(p => ({
          id: `custom-${p.processId}`,
          type: 'custom' as const,
          label: p.scriptName ?? 'Script',
          processId: p.processId,
        }));

      if (newTerminals.length === 0) return prev;
      return [...prev, ...newTerminals];
    });
  }, [sessionId, customProcesses]);

  // Load output for custom terminals that are no longer running (restore after page refresh)
  // Running processes get output via WebSocket streaming - don't fetch from API for those
  useEffect(() => {
    if (!sessionId) return;

    const customTerminals = terminals.filter(t => t.type === 'custom' && t.processId);

    customTerminals.forEach(async (terminal) => {
      const processId = terminal.processId!;
      if (loadedOutputsRef.current.has(processId)) return;

      // Check process status - only fetch from API for completed/stopped/failed processes
      // Running/starting processes get output via WebSocket streaming
      const processInfo = processState.processesById[processId];
      const isActive = !processInfo || processInfo.status === 'running' || processInfo.status === 'starting';
      if (isActive) return;

      // Process is completed/stopped/failed - check if we need to restore output from API
      // Get current output from store (don't use as dependency to avoid re-runs on every output change)
      const currentOutput = storeApi.getState().processes.outputsByProcessId[processId];
      if (currentOutput && currentOutput.length > 0) {
        loadedOutputsRef.current.add(processId);
        return;
      }

      loadedOutputsRef.current.add(processId);

      try {
        const fullProc = await getSessionProcess(sessionId, processId, true);
        clearProcessOutput(processId);
        if (fullProc.command) {
          appendProcessOutput(processId, `$ ${fullProc.command}\n\n`);
        }
        fullProc.output?.forEach(chunk => {
          appendProcessOutput(processId, chunk.data);
        });
      } catch {
        // Remove from loaded set on error so it can be retried
        loadedOutputsRef.current.delete(processId);
      }
    });
  }, [sessionId, terminals, processState.processesById, clearProcessOutput, appendProcessOutput, storeApi]);

  // Restore saved custom tab after terminals are set up
  useEffect(() => {
    if (!sessionId || tabRestoredRef.current) return;

    const savedTab = getLocalStorage<string | null>(`rightPanel-tab-${sessionId}`, null);
    if (!savedTab?.startsWith('custom-')) return;

    // Check if this custom tab exists in terminals
    const terminalExists = terminals.some(t => t.id === savedTab);
    if (terminalExists) {
      setRightPanelActiveTab(sessionId, savedTab);
      tabRestoredRef.current = true;
    }
  }, [sessionId, terminals, setRightPanelActiveTab]);

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
        const terminalToRemove = prev.find((t) => t.id === id);
        if (!terminalToRemove) return prev;

        // Only enforce "keep at least one shell" for shell terminals
        if (terminalToRemove.type === 'shell') {
          const shellTerminals = prev.filter((t) => t.type === 'shell');
          if (shellTerminals.length <= 1) {
            return prev; // Keep at least one shell terminal
          }
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

  // Save collapse state to local storage
  useEffect(() => {
    setLocalStorage('task-right-panel-collapsed', isBottomCollapsed);
  }, [isBottomCollapsed]);

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

  const handleRunCommand = useCallback(async (script: RepositoryScript) => {
    if (!sessionId) return;

    try {
      const response = await startProcess(sessionId, {
        kind: 'custom',
        script_name: script.name
      });

      if (response?.process) {
        const newTerminal: Terminal = {
          id: `custom-${response.process.id}`,
          type: 'custom',
          label: script.name,
          processId: response.process.id,
        };

        // Mark as loaded immediately - WebSocket will stream output, no need to fetch from API
        loadedOutputsRef.current.add(response.process.id);

        // Display the command for processes that are still starting/running
        // Completed/stopped/failed processes will have output restored by the effect (avoids race condition)
        const status = response.process.status;
        if (response.process.command && (status === 'starting' || status === 'running')) {
          clearProcessOutput(response.process.id);
          appendProcessOutput(response.process.id, `$ ${response.process.command}\n\n`);
        }

        setTerminals(prev => {
          // Check if terminal already exists
          if (prev.some(t => t.id === newTerminal.id)) {
            return prev;
          }
          return [...prev, newTerminal];
        });
        setRightPanelActiveTab(sessionId, newTerminal.id);

        // Update store with process status
        upsertProcessStatus({
          processId: response.process.id,
          sessionId: response.process.session_id,
          kind: response.process.kind,
          scriptName: response.process.script_name,
          status: response.process.status,
          command: response.process.command,
          workingDir: response.process.working_dir,
          exitCode: response.process.exit_code ?? null,
          startedAt: response.process.started_at,
          updatedAt: response.process.updated_at,
        });
      }
    } catch (error) {
      console.error('Failed to start command:', error);
    }
  }, [sessionId, setRightPanelActiveTab, upsertProcessStatus, clearProcessOutput, appendProcessOutput]);

  const handleCloseCustomTab = useCallback(async (
    event: MouseEvent,
    terminalId: string,
    processId?: string
  ) => {
    event.preventDefault();
    event.stopPropagation();

    // Mark as closed so sync effect doesn't re-add it
    if (processId) {
      closedProcessIdsRef.current.add(processId);
    }

    // Remove the terminal immediately
    removeTerminal(terminalId);

    // Stop process if still running (don't await - do it in background)
    if (processId && sessionId) {
      const proc = processState.processesById[processId];
      if (proc?.status === 'running' || proc?.status === 'starting') {
        stopProcess(sessionId, { process_id: processId }).catch((error) => {
          console.error('Failed to stop process:', error);
        });
      }
    }
  }, [sessionId, processState.processesById, removeTerminal]);

  const tabs: SessionTab[] = useMemo(() => {
    const allTabs: SessionTab[] = [];

    // Only show Commands tab if scripts exist (from SSR or loaded)
    if (hasScripts) {
      allTabs.push({ id: 'commands', label: 'Commands' });
    }

    // Add terminal tabs (dev-server, shell, custom)
    const terminalTabs: SessionTab[] = terminals.map((terminal) => {
      const shellCount = terminals.filter((t) => t.type === 'shell').length;
      const isClosable =
        terminal.type === 'dev-server' ||
        terminal.type === 'custom' ||
        (terminal.type === 'shell' && shellCount > 1);

      // Check if custom process is running
      let icon: React.ReactNode = undefined;
      if (terminal.type === 'custom' && terminal.processId) {
        const proc = processState.processesById[terminal.processId];
        const isRunning = proc?.status === 'running' || proc?.status === 'starting';
        if (isRunning) {
          icon = <span className="w-1.5 h-1.5 rounded-full bg-green-500" />;
        }
      }

      return {
        id: terminal.id,
        label: terminal.label,
        icon,
        className: isClosable
          ? 'group flex items-center gap-1 pr-1 cursor-pointer'
          : 'cursor-pointer',
        closable: isClosable,
        alwaysShowClose: terminal.type === 'custom',
        onClose:
          terminal.type === 'dev-server'
            ? handleCloseDevTab
            : terminal.type === 'custom'
              ? (e: MouseEvent) => handleCloseCustomTab(e, terminal.id, terminal.processId)
              : (e: MouseEvent) => {
                e.preventDefault();
                e.stopPropagation();
                removeTerminal(terminal.id);
              },
      };
    });

    return [...allTabs, ...terminalTabs];
  }, [hasScripts, terminals, processState.processesById, handleCloseDevTab, handleCloseCustomTab, removeTerminal]);

  if (isBottomCollapsed) {
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
            onTabChange={(value) => {
              if (sessionId) {
                setRightPanelActiveTab(sessionId, value);
              }
            }}
            showAddButton
            onAddTab={addTerminal}
            collapsible
            isCollapsed={isBottomCollapsed}
            onToggleCollapse={() => setIsBottomCollapsed(false)}
            className="flex-1 min-h-0"
          />
        </SessionPanel>
      </div>
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
            onTabChange={(value) => {
              if (sessionId) {
                setRightPanelActiveTab(sessionId, value);
              }
            }}
            showAddButton
            onAddTab={addTerminal}
            collapsible
            isCollapsed={isBottomCollapsed}
            onToggleCollapse={() => setIsBottomCollapsed(true)}
            className="flex-1 min-h-0"
          >
            <TabsContent value="commands" className="flex-1 min-h-0">
              <SessionPanelContent>
                <div className="grid gap-2">
                  {scripts.map((script) => (
                    <button
                      key={script.id}
                      type="button"
                      onClick={() => handleRunCommand(script)}
                      className="flex items-center gap-2 rounded-md border border-border px-3 py-2 text-sm text-left hover:bg-muted cursor-pointer min-w-0"
                    >
                      <span className="flex-1 min-w-0 truncate text-xs">
                        {script.name}
                      </span>
                      <Badge
                        variant="secondary"
                        className="shrink-0 font-mono text-xs max-w-[60%] min-w-0"
                      >
                        <span className="truncate block">
                          {script.command}
                        </span>
                      </Badge>
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
                      key={devProcessId}
                      processOutput={devOutput}
                      processId={devProcessId ?? null}
                      isStopping={isStoppingDev}
                    />
                  ) : terminal.type === 'custom' ? (
                    <ShellTerminal
                      key={terminal.processId}
                      processOutput={processState.outputsByProcessId[terminal.processId ?? ''] ?? ''}
                      processId={terminal.processId ?? null}
                    />
                  ) : (
                    <ShellTerminal key={sessionId} sessionId={sessionId ?? undefined} />
                  )}
                </SessionPanelContent>
              </TabsContent>
            ))}
          </SessionTabs>
        </SessionPanel>
      </Panel>
    </Group>
  );
});

export { TaskRightPanel };
