'use client';

import { useState, useEffect } from 'react';
import { IconRefresh } from '@tabler/icons-react';
import { Button } from '@kandev/ui/button';
import { Input } from '@kandev/ui/input';
import { usePreviewPanel } from '@/hooks/use-preview-panel';
import { useAppStore, useAppStoreApi } from '@/components/state-provider';
import { useLayoutStore } from '@/lib/state/layout-store';
import { ProcessOutputTerminal } from '@/components/task/process-output-terminal';
import { getLocalStorage } from '@/lib/local-storage';

type PreviewPanelProps = {
  sessionId: string | null;
  hasDevScript: boolean;
};

export function PreviewPanel({ sessionId, hasDevScript }: PreviewPanelProps) {
  const {
    previewUrl,
    previewUrlDraft,
    setPreviewUrl,
    setPreviewUrlDraft,
    isStopping,
    handleStop,
  } = usePreviewPanel({ sessionId, hasDevScript });
  const closeLayoutPreview = useLayoutStore((state) => state.closePreview);
  const setPreviewOpen = useAppStore((state) => state.setPreviewOpen);
  const setPreviewStage = useAppStore((state) => state.setPreviewStage);
  const setPreviewView = useAppStore((state) => state.setPreviewView);
  const clearProcessOutput = useAppStore((state) => state.clearProcessOutput);
  const appStoreApi = useAppStoreApi();
  const previewView = useAppStore((state) => {
    if (!sessionId) return 'preview';
    const stored = state.previewPanel.viewBySessionId[sessionId];
    if (stored) return stored;

    // Try to get from localStorage on first render
    const key = `preview-view:${sessionId}`;
    const localStored = getLocalStorage(key, null as string | null);
    if (localStored === 'output' || localStored === 'preview') {
      return localStored;
    }

    return 'preview';
  });
  const devProcessId = useAppStore((state) =>
    sessionId ? state.processes.devProcessBySessionId[sessionId] : undefined
  );
  const devProcess = useAppStore((state) =>
    devProcessId ? state.processes.processesById[devProcessId] : undefined
  );
  const devOutput = useAppStore ((state) =>
    devProcessId ? state.processes.outputsByProcessId[devProcessId] ?? '' : ''
  );
  const [refreshKey, setRefreshKey] = useState(0);

  const isProcessFailed = devProcess?.status === 'failed';
  const isProcessExited = devProcess?.status === 'exited';

  // Sync the view from localStorage to store if needed
  useEffect(() => {
    if (!sessionId) return;
    const storeView = appStoreApi.getState().previewPanel.viewBySessionId[sessionId];
    if (storeView) return; // Already set in store

    const key = `preview-view:${sessionId}`;
    const stored = getLocalStorage(key, null as string | null);
    if (stored === 'output' || stored === 'preview') {
      setPreviewView(sessionId, stored);
    }
  }, [sessionId, appStoreApi, setPreviewView]);

  if (!sessionId) {
    return (
      <div className="h-full w-full flex items-center justify-center text-muted-foreground mr-[5px]">
        Select a session to enable preview.
      </div>
    );
  }

  if (!hasDevScript) {
    return (
      <div className="h-full w-full flex items-center justify-center text-muted-foreground mr-[5px]">
        Configure a dev script to use preview.
      </div>
    );
  }

  const handleStopClick = async () => {
    if (!sessionId) return;
    setPreviewOpen(sessionId, false);
    setPreviewStage(sessionId, 'closed');
    setPreviewView(sessionId, 'preview');
    setPreviewUrl(sessionId, '');
    setPreviewUrlDraft(sessionId, '');
    closeLayoutPreview(sessionId);
    if (devProcessId) {
      clearProcessOutput(devProcessId);
    }
    await handleStop();
  };

  return (
    <div className="h-full min-h-0 flex flex-col rounded-lg border border-border/70 bg-card p-3 mr-[5px]">
      <div className="flex flex-wrap items-center gap-2">
        <Input
          value={previewUrlDraft}
          onChange={(event) => setPreviewUrlDraft(sessionId, event.target.value)}
          onBlur={() => setPreviewUrl(sessionId, previewUrlDraft)}
          onKeyDown={(event) => {
            if (event.key === 'Enter') {
              event.preventDefault();
              setPreviewUrl(sessionId, previewUrlDraft);
            }
          }}
          placeholder="http://localhost:3000"
          className="h-8 flex-1 min-w-[240px]"
        />
        <Button
          size="sm"
          variant="outline"
          onClick={() => setRefreshKey((value) => value + 1)}
          disabled={!previewUrl}
          className="cursor-pointer"
        >
          <IconRefresh className="h-4 w-4" />
        </Button>
        <Button
          size="sm"
          variant="outline"
          onClick={handleStopClick}
          disabled={isStopping}
          className="cursor-pointer"
        >
          {isStopping ? 'Stopping…' : (isProcessFailed || isProcessExited) ? 'Close' : 'Stop'}
        </Button>
        <Button
          size="sm"
          variant={previewView === 'output' ? 'default' : 'outline'}
          className="cursor-pointer"
          onClick={() =>
            setPreviewView(sessionId, previewView === 'output' ? 'preview' : 'output')
          }
        >
          {previewView === 'output' ? 'Preview' : 'Logs'}
        </Button>
      </div>

      <div className="flex-1 min-h-0 mt-3">
        {previewView === 'output' ? (
          <div className="h-full w-full">
            <ProcessOutputTerminal output={devOutput} processId={devProcessId ?? null} />
          </div>
        ) : previewUrl ? (
          <div className="h-full w-full flex items-center justify-center overflow-hidden">
            <div className="h-full w-full bg-background border border-border rounded-md overflow-hidden">
              <iframe
                key={refreshKey}
                src={previewUrl}
                title="Preview"
                className="h-full w-full border-0"
                sandbox="allow-scripts allow-same-origin allow-forms allow-popups allow-modals"
                referrerPolicy="no-referrer"
              />
            </div>
          </div>
        ) : (
          <div className="h-full w-full flex items-center justify-center text-muted-foreground">
            Waiting for a preview URL…
          </div>
        )}
      </div>
    </div>
  );
}
