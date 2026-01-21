'use client';

import { useEffect, useMemo, useState } from 'react';
import { getSessionProcess, listSessionProcesses, startProcess, stopProcess } from '@/lib/http';
import { useAppStore } from '@/components/state-provider';
import { useLayoutStore } from '@/lib/state/layout-store';
import type { ProcessInfo } from '@/lib/types/http';
import type { ProcessStatusEntry } from '@/lib/state/store';
import { getLocalStorage } from '@/lib/local-storage';

type UsePreviewPanelParams = {
  sessionId: string | null;
  hasDevScript?: boolean;
};

function toProcessStatusEntry(process: ProcessInfo): ProcessStatusEntry {
  return {
    processId: process.id,
    sessionId: process.session_id,
    kind: process.kind,
    scriptName: process.script_name,
    status: process.status,
    command: process.command,
    workingDir: process.working_dir,
    exitCode: process.exit_code ?? null,
    startedAt: process.started_at,
    updatedAt: process.updated_at,
  };
}

export function usePreviewPanel({ sessionId, hasDevScript = false }: UsePreviewPanelParams) {
  const [isStopping, setIsStopping] = useState(false);
  const [hasInitialized, setHasInitialized] = useState(false);
  const [hasRestoredState, setHasRestoredState] = useState(false);
  const [isStartingOnRestore, setIsStartingOnRestore] = useState(false);

  const processState = useAppStore((state) => state.processes);
  const previewOpen = useAppStore((state) =>
    sessionId ? state.previewPanel.openBySessionId[sessionId] ?? false : false
  );
  const previewStage = useAppStore((state) =>
    sessionId ? state.previewPanel.stageBySessionId[sessionId] ?? 'closed' : 'closed'
  );
  const previewUrl = useAppStore((state) =>
    sessionId ? state.previewPanel.urlBySessionId[sessionId] ?? '' : ''
  );
  const previewUrlDraft = useAppStore((state) =>
    sessionId ? state.previewPanel.urlDraftBySessionId[sessionId] ?? '' : ''
  );
  const previewView = useAppStore((state) =>
    sessionId ? state.previewPanel.viewBySessionId[sessionId] ?? null : null
  );
  const upsertProcessStatus = useAppStore((state) => state.upsertProcessStatus);
  const appendProcessOutput = useAppStore((state) => state.appendProcessOutput);
  const clearProcessOutput = useAppStore((state) => state.clearProcessOutput);
  const setActiveProcess = useAppStore((state) => state.setActiveProcess);
  const setPreviewOpen = useAppStore((state) => state.setPreviewOpen);
  const setPreviewStage = useAppStore((state) => state.setPreviewStage);
  const setPreviewView = useAppStore((state) => state.setPreviewView);
  const applyLayoutPreset = useLayoutStore((state) => state.applyPreset);
  const layoutState = useLayoutStore((state) =>
    sessionId ? state.columnsBySessionId[sessionId] : null
  );
  const setPreviewUrl = useAppStore((state) => state.setPreviewUrl);
  const setPreviewUrlDraft = useAppStore((state) => state.setPreviewUrlDraft);

  const devProcessId = useMemo(
    () => (sessionId ? processState.devProcessBySessionId[sessionId] : undefined),
    [processState.devProcessBySessionId, sessionId]
  );

  const devProcess = devProcessId ? processState.processesById[devProcessId] ?? null : null;
  const devOutput = devProcessId ? processState.outputsByProcessId[devProcessId] ?? '' : '';

  const detectedUrl = useMemo(() => {
    if (!devOutput) return null;
    const matches = devOutput.match(/https?:\/\/(?:localhost|127\.0\.0\.1)(?::\d+)?[^\s]*/g);
    if (!matches || matches.length === 0) return null;
    return matches[matches.length - 1];
  }, [devOutput]);

  useEffect(() => {
    if (!sessionId) return;
    if (!previewOpen || previewStage !== 'logs') return;
    if (!detectedUrl) {
      console.log('preview-panel:url-not-detected', {
        sessionId,
        previewStage,
        previewOpen,
        outputLength: devOutput.length,
        outputTail: devOutput.slice(-500),
      });
      return;
    }
    console.log('preview-panel:url-detected', {
      sessionId,
      previewStage,
      previewOpen,
      detectedUrl,
    });
    setPreviewUrl(sessionId, detectedUrl);
    setPreviewUrlDraft(sessionId, detectedUrl);
    setPreviewView(sessionId, 'preview');
    setPreviewStage(sessionId, 'preview');
    applyLayoutPreset(sessionId, 'preview');
  }, [
    sessionId,
    previewOpen,
    previewStage,
    detectedUrl,
    setPreviewUrl,
    setPreviewUrlDraft,
    setPreviewView,
    setPreviewStage,
    applyLayoutPreset,
  ]);

  useEffect(() => {
    if (!sessionId) return;
    let cancelled = false;
    const loadProcesses = async () => {
      try {
        const processes = await listSessionProcesses(sessionId);
        if (cancelled) return;
        const sorted = processes.slice().sort((a, b) => {
          if (a.kind === 'dev' && b.kind !== 'dev') return -1;
          if (a.kind !== 'dev' && b.kind === 'dev') return 1;
          return 0;
        });
        sorted.forEach((proc) => {
          upsertProcessStatus(toProcessStatusEntry(proc));
        });
        setHasInitialized(true);
      } catch {
        // ignore
        setHasInitialized(true);
      }
    };
    loadProcesses();
    return () => {
      cancelled = true;
    };
  }, [sessionId, upsertProcessStatus]);

  useEffect(() => {
    if (!sessionId || !devProcessId) return;
    let cancelled = false;
    const loadOutput = async () => {
      try {
        const proc = await getSessionProcess(sessionId, devProcessId, true);
        if (cancelled) return;
        clearProcessOutput(devProcessId);
        proc.output?.forEach((chunk) => {
          appendProcessOutput(devProcessId, chunk.data);
        });
      } catch {
        // ignore
      }
    };
    loadOutput();
    return () => {
      cancelled = true;
    };
  }, [sessionId, devProcessId, clearProcessOutput, appendProcessOutput]);

  // Restore preview state on mount if layout shows preview and dev process is running
  useEffect(() => {
    if (!sessionId || !hasInitialized || hasRestoredState || isStartingOnRestore) return;

    const isPreviewLayout = layoutState?.preview === true;
    const hasDevProcess = devProcessId !== undefined;
    const isDevRunning = devProcess?.status === 'running' || devProcess?.status === 'starting';

    // Restore the preview view mode from localStorage
    const key = `preview-view:${sessionId}`;
    const stored = getLocalStorage(key, null as string | null);
    const restoredView: 'output' | 'preview' =
      stored === 'output' || stored === 'preview' ? stored : 'output';

    if (isPreviewLayout) {
      if (hasDevProcess && isDevRunning) {
        // Layout shows preview and dev is running - restore everything
        if (!previewOpen) {
          setPreviewOpen(sessionId, true);
        }
        setPreviewView(sessionId, restoredView);
        if (previewStage === 'closed') {
          setPreviewStage(sessionId, 'preview');
        }
        setHasRestoredState(true);
      } else if (!hasDevProcess && hasDevScript) {
        // Layout shows preview but no process - start one
        setIsStartingOnRestore(true);
        setPreviewOpen(sessionId, true);
        setPreviewView(sessionId, restoredView);
        setPreviewStage(sessionId, 'preview');

        startProcess(sessionId, { kind: 'dev' })
          .then((resp) => {
            if (resp?.process) {
              const status = {
                processId: resp.process.id,
                sessionId: resp.process.session_id,
                kind: resp.process.kind,
                scriptName: resp.process.script_name,
                status: resp.process.status,
                command: resp.process.command,
                workingDir: resp.process.working_dir,
                exitCode: resp.process.exit_code ?? null,
                startedAt: resp.process.started_at,
                updatedAt: resp.process.updated_at,
              };
              upsertProcessStatus(status);
              setActiveProcess(status.sessionId, status.processId);
            }
          })
          .finally(() => {
            setIsStartingOnRestore(false);
            setHasRestoredState(true);
          });
      } else {
        // Layout shows preview but process exists and isn't running - still restore view
        setPreviewView(sessionId, restoredView);
        setHasRestoredState(true);
      }
    } else {
      // No preview in layout - mark as restored so we don't keep checking
      setHasRestoredState(true);
    }
  }, [
    sessionId,
    hasInitialized,
    hasRestoredState,
    isStartingOnRestore,
    previewOpen,
    layoutState,
    devProcessId,
    devProcess,
    previewStage,
    hasDevScript,
    setPreviewOpen,
    setPreviewView,
    setPreviewStage,
    upsertProcessStatus,
    setActiveProcess,
  ]);

  useEffect(() => {
    if (!sessionId) return;
    if (!detectedUrl) return;
    if (!previewOpen && previewStage === 'closed') {
      return;
    }
    if (!previewOpen) {
      setPreviewOpen(sessionId, true);
    }
    if (!previewUrl) {
      setPreviewUrl(sessionId, detectedUrl);
      setPreviewUrlDraft(sessionId, detectedUrl);
    }
  }, [
    sessionId,
    previewOpen,
    detectedUrl,
    previewUrl,
    previewStage,
    setPreviewOpen,
    setPreviewUrl,
    setPreviewUrlDraft,
  ]);

  const handleStop = async () => {
    if (!sessionId || !devProcess) return;
    setIsStopping(true);
    try {
      await stopProcess(sessionId, { process_id: devProcess.processId });
    } finally {
      setIsStopping(false);
    }
  };

  const isRunning = devProcess?.status === 'running' || devProcess?.status === 'starting';

  return {
    previewStage,
    previewUrl,
    previewUrlDraft,
    setPreviewUrl,
    setPreviewUrlDraft,
    detectedUrl,
    isRunning,
    isStopping,
    handleStop,
  };
}
