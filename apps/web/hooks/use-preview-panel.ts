'use client';

import { useCallback, useEffect, useMemo, useState } from 'react';
import { getSessionProcess, listSessionProcesses, startProcess, stopProcess } from '@/lib/api';
import { useAppStore } from '@/components/state-provider';
import { useLayoutStore } from '@/lib/state/layout-store';
import type { ProcessInfo } from '@/lib/types/http';
import type { ProcessStatusEntry } from '@/lib/state/store';
import { getLocalStorage } from '@/lib/local-storage';
import { detectPreviewUrlFromOutput } from '@/lib/preview-url-detector';

type UsePreviewPanelParams = {
    sessionId: string | null;
    hasDevScript?: boolean;
};

function toProcessStatusEntry(process: ProcessInfo): ProcessStatusEntry {
    return {
        processId: process.id, sessionId: process.session_id, kind: process.kind,
        scriptName: process.script_name, status: process.status, command: process.command,
        workingDir: process.working_dir, exitCode: process.exit_code ?? null,
        startedAt: process.started_at, updatedAt: process.updated_at,
    };
}

function sortProcesses(processes: ProcessInfo[]): ProcessInfo[] {
    return processes.slice().sort((a, b) => {
        if (a.kind === 'dev' && b.kind !== 'dev') return -1;
        if (a.kind !== 'dev' && b.kind === 'dev') return 1;
        return 0;
    });
}

function useProcessLoader(sessionId: string | null, upsertProcessStatus: (s: ProcessStatusEntry) => void) {
    const [hasInitialized, setHasInitialized] = useState(false);
    useEffect(() => {
        if (!sessionId) return;
        let cancelled = false;
        listSessionProcesses(sessionId)
            .then((processes) => {
                if (cancelled) return;
                sortProcesses(processes).forEach((proc) => { upsertProcessStatus(toProcessStatusEntry(proc)); });
                setHasInitialized(true);
            })
            .catch(() => { setHasInitialized(true); });
        return () => { cancelled = true; };
    }, [sessionId, upsertProcessStatus]);
    return hasInitialized;
}

function useDevOutputLoader(
    sessionId: string | null, devProcessId: string | undefined,
    clearProcessOutput: (id: string) => void, appendProcessOutput: (id: string, data: string) => void,
) {
    useEffect(() => {
        if (!sessionId || !devProcessId) return;
        let cancelled = false;
        getSessionProcess(sessionId, devProcessId, true)
            .then((proc) => {
                if (cancelled) return;
                clearProcessOutput(devProcessId);
                if (proc.command) appendProcessOutput(devProcessId, `${proc.command}\n\n`);
                proc.output?.forEach((chunk) => { appendProcessOutput(devProcessId, chunk.data); });
            })
            .catch(() => { /* ignore */ });
        return () => { cancelled = true; };
    }, [sessionId, devProcessId, clearProcessOutput, appendProcessOutput]);
}

function resolveRestoreView(sessionId: string): 'output' | 'preview' {
    const stored = getLocalStorage(`preview-view:${sessionId}`, null as string | null);
    return stored === 'output' || stored === 'preview' ? stored : 'output';
}

function usePreviewStore(sessionId: string | null) {
    const previewOpen = useAppStore((s) => sessionId ? s.previewPanel.openBySessionId[sessionId] ?? false : false);
    const previewStage = useAppStore((s) => sessionId ? s.previewPanel.stageBySessionId[sessionId] ?? 'closed' : 'closed');
    const previewUrl = useAppStore((s) => sessionId ? s.previewPanel.urlBySessionId[sessionId] ?? '' : '');
    const previewUrlDraft = useAppStore((s) => sessionId ? s.previewPanel.urlDraftBySessionId[sessionId] ?? '' : '');
    const setPreviewOpen = useAppStore((s) => s.setPreviewOpen);
    const setPreviewStage = useAppStore((s) => s.setPreviewStage);
    const setPreviewView = useAppStore((s) => s.setPreviewView);
    const setPreviewUrl = useAppStore((s) => s.setPreviewUrl);
    const setPreviewUrlDraft = useAppStore((s) => s.setPreviewUrlDraft);
    return { previewOpen, previewStage, previewUrl, previewUrlDraft, setPreviewOpen, setPreviewStage, setPreviewView, setPreviewUrl, setPreviewUrlDraft };
}

export function usePreviewPanel({ sessionId, hasDevScript = false }: UsePreviewPanelParams) {
    const [isStopping, setIsStopping] = useState(false);
    const [hasRestoredState, setHasRestoredState] = useState(false);
    const [isStartingOnRestore, setIsStartingOnRestore] = useState(false);

    const processState = useAppStore((state) => state.processes);
    const upsertProcessStatus = useAppStore((state) => state.upsertProcessStatus);
    const appendProcessOutput = useAppStore((state) => state.appendProcessOutput);
    const clearProcessOutput = useAppStore((state) => state.clearProcessOutput);
    const setActiveProcess = useAppStore((state) => state.setActiveProcess);
    const applyLayoutPreset = useLayoutStore((state) => state.applyPreset);
    const layoutState = useLayoutStore((state) => sessionId ? state.columnsBySessionId[sessionId] : null);

    const { previewOpen, previewStage, previewUrl, previewUrlDraft, setPreviewOpen, setPreviewStage, setPreviewView, setPreviewUrl, setPreviewUrlDraft } = usePreviewStore(sessionId);

    const devProcessId = useMemo(
        () => (sessionId ? processState.devProcessBySessionId[sessionId] : undefined),
        [processState.devProcessBySessionId, sessionId]
    );
    const devProcess = devProcessId ? processState.processesById[devProcessId] ?? null : null;
    const devOutput = devProcessId ? processState.outputsByProcessId[devProcessId] ?? '' : '';
    const detectedUrl = useMemo(() => detectPreviewUrlFromOutput(devOutput), [devOutput]);

    const hasInitialized = useProcessLoader(sessionId, upsertProcessStatus);
    useDevOutputLoader(sessionId, devProcessId, clearProcessOutput, appendProcessOutput);

    useEffect(() => {
        if (!sessionId || !previewOpen || previewStage !== 'logs' || !detectedUrl) return;
        setPreviewUrl(sessionId, detectedUrl);
        setPreviewUrlDraft(sessionId, detectedUrl);
        setPreviewView(sessionId, 'preview');
        setPreviewStage(sessionId, 'preview');
        applyLayoutPreset(sessionId, 'preview');
    }, [sessionId, previewOpen, previewStage, detectedUrl, setPreviewUrl, setPreviewUrlDraft, setPreviewView, setPreviewStage, applyLayoutPreset]);

    const restoreRunningPreview = useCallback((sid: string, view: 'output' | 'preview') => {
        if (!previewOpen) setPreviewOpen(sid, true);
        setPreviewView(sid, view);
        if (previewStage === 'closed') setPreviewStage(sid, 'preview');
        setHasRestoredState(true);
    }, [previewOpen, previewStage, setPreviewOpen, setPreviewView, setPreviewStage]);

    const startDevAndRestore = useCallback((sid: string, view: 'output' | 'preview') => {
        setIsStartingOnRestore(true);
        setPreviewOpen(sid, true);
        setPreviewView(sid, view);
        setPreviewStage(sid, 'preview');
        startProcess(sid, { kind: 'dev' })
            .then((resp) => {
                if (!resp?.process) return;
                const p = resp.process;
                const status = {
                    processId: p.id, sessionId: p.session_id, kind: p.kind,
                    scriptName: p.script_name, status: p.status, command: p.command,
                    workingDir: p.working_dir, exitCode: p.exit_code ?? null,
                    startedAt: p.started_at, updatedAt: p.updated_at,
                };
                upsertProcessStatus(status);
                setActiveProcess(status.sessionId, status.processId);
            })
            .finally(() => { setIsStartingOnRestore(false); setHasRestoredState(true); });
    }, [setPreviewOpen, setPreviewView, setPreviewStage, upsertProcessStatus, setActiveProcess]);

    useEffect(() => {
        if (!sessionId || !hasInitialized || hasRestoredState || isStartingOnRestore) return;
        if (!layoutState?.preview) { setHasRestoredState(true); return; }
        const restoredView = resolveRestoreView(sessionId);
        const isDevRunning = devProcess?.status === 'running' || devProcess?.status === 'starting';
        if (devProcessId !== undefined && isDevRunning) {
            restoreRunningPreview(sessionId, restoredView);
        } else if (devProcessId === undefined && hasDevScript) {
            startDevAndRestore(sessionId, restoredView);
        } else {
            setPreviewView(sessionId, restoredView);
            setHasRestoredState(true);
        }
    }, [sessionId, hasInitialized, hasRestoredState, isStartingOnRestore, layoutState, devProcessId, devProcess, hasDevScript, restoreRunningPreview, startDevAndRestore, setPreviewView]);

    useEffect(() => {
        if (!sessionId || !detectedUrl) return;
        if (!previewOpen && previewStage === 'closed') return;
        if (!previewOpen) setPreviewOpen(sessionId, true);
        if (!previewUrl) {
            setPreviewUrl(sessionId, detectedUrl);
            setPreviewUrlDraft(sessionId, detectedUrl);
        }
    }, [sessionId, previewOpen, detectedUrl, previewUrl, previewStage, setPreviewOpen, setPreviewUrl, setPreviewUrlDraft]);

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
        previewStage, previewUrl, previewUrlDraft, setPreviewUrl, setPreviewUrlDraft,
        detectedUrl, isRunning, isStopping, handleStop,
    };
}
