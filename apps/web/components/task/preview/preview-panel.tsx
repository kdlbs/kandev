'use client';

import { useState, useEffect } from 'react';
import { IconRefresh, IconExternalLink, IconLoader2 } from '@tabler/icons-react';
import { Button } from '@kandev/ui/button';
import { Input } from '@kandev/ui/input';
import { SessionPanel, SessionPanelContent } from '@kandev/ui/pannel-session';
import { usePreviewPanel } from '@/hooks/use-preview-panel';
import { useAppStore, useAppStoreApi } from '@/components/state-provider';
import { ShellTerminal } from '@/components/task/shell-terminal';
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
    detectedUrl,
    isRunning,
  } = usePreviewPanel({ sessionId, hasDevScript });
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
  const devOutput = useAppStore((state) =>
    devProcessId ? state.processes.outputsByProcessId[devProcessId] ?? '' : ''
  );
  const [refreshKey, setRefreshKey] = useState(0);
  const [showIframe, setShowIframe] = useState(false);
  const [allowManualUrl, setAllowManualUrl] = useState(false);
  const [showLoadingSpinner, setShowLoadingSpinner] = useState(true);

  const isProcessFailed = devProcess?.status === 'failed';
  const isProcessExited = devProcess?.status === 'exited';

  // Check if we're waiting for a URL (for button spinner)
  // Show spinner on "Preview" button when on output view and waiting for URL
  const isWaitingForUrl = isRunning && previewView === 'output' && !previewUrl && showLoadingSpinner;

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

  // 10-second timeout to enable manual URL entry and stop loading spinner when no URL detected
  useEffect(() => {
    if (!isRunning || detectedUrl) {
      // Clear immediately using setTimeout to avoid synchronous setState in effect
      const immediate = setTimeout(() => {
        setAllowManualUrl(false);
        setShowLoadingSpinner(true);
      }, 0);
      return () => clearTimeout(immediate);
    }

    // Reset loading spinner when starting to wait
    const resetTimer = setTimeout(() => setShowLoadingSpinner(true), 0);

    // After 10 seconds, enable manual URL, stop showing loading spinner, and switch to preview view
    const timer = setTimeout(() => {
      setAllowManualUrl(true);
      setShowLoadingSpinner(false);
      // Switch to preview view to show the "No URL detected" message
      if (sessionId) {
        setPreviewView(sessionId, 'preview');
      }
    }, 10000);

    return () => {
      clearTimeout(resetTimer);
      clearTimeout(timer);
    };
  }, [isRunning, detectedUrl, sessionId, setPreviewView]);

  // 2-second delay before showing iframe after URL detection
  useEffect(() => {
    if (!previewUrl) {
      // Clear immediately using setTimeout to avoid synchronous setState in effect
      const immediate = setTimeout(() => setShowIframe(false), 0);
      return () => clearTimeout(immediate);
    }

    // Start hidden, then show after delay
    const hideTimer = setTimeout(() => setShowIframe(false), 0);
    const showTimer = setTimeout(() => setShowIframe(true), 2000);

    return () => {
      clearTimeout(hideTimer);
      clearTimeout(showTimer);
    };
  }, [previewUrl, refreshKey]);

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

    if (devProcessId) {
      clearProcessOutput(devProcessId);
    }
    await handleStop();
  };

  const handleUrlSubmit = () => {
    if (!sessionId) return;
    const trimmed = previewUrlDraft.trim();
    if (trimmed) {
      setPreviewUrl(sessionId, trimmed);
    }
  };

  const handleOpenInTab = () => {
    if (!sessionId || !previewUrl) return;

    // Open URL in new tab
    window.open(previewUrl, '_blank', 'noopener,noreferrer');

    // Close the preview panel but keep dev server running
    setPreviewOpen(sessionId, false);
    setPreviewStage(sessionId, 'closed');
    setPreviewView(sessionId, 'preview');
    setPreviewUrl(sessionId, '');
    setPreviewUrlDraft(sessionId, '');

  };

  return (
    <SessionPanel margin="right">
      <div className="h-full flex flex-col gap-2 flex-1">
        <div className="flex flex-wrap items-center gap-2">
          <Input
            value={previewUrlDraft}
            onChange={(event) => setPreviewUrlDraft(sessionId, event.target.value)}
            onKeyDown={(event) => {
              if (event.key === 'Enter') {
                event.preventDefault();
                handleUrlSubmit();
              }
            }}
            placeholder={detectedUrl || "http://localhost:3000"}
            className="h-6 flex-1 min-w-[240px]"
          />
          <Button
            size="sm"
            variant="outline"
            onClick={handleOpenInTab}
            disabled={!previewUrl}
            className="cursor-pointer"
            title="Open in browser tab"
          >
            <IconExternalLink className="h-4 w-4" />
          </Button>
          <Button
            size="sm"
            variant="outline"
            onClick={() => setRefreshKey((value) => value + 1)}
            disabled={!previewUrl}
            className="cursor-pointer"
            title="Refresh preview"
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
            {isWaitingForUrl && (
              <IconLoader2 className="h-4 w-4 mr-1 animate-spin" />
            )}
            {previewView === 'output' ? 'Preview' : 'Logs'}
          </Button>
        </div>

        <SessionPanelContent className={previewView === 'output' || (showIframe && previewUrl) ? 'p-0' : ''}>
          {previewView === 'output' ? (
            <ShellTerminal processOutput={devOutput} processId={devProcessId ?? null} />
          ) : showIframe && previewUrl ? (
            <iframe
              key={refreshKey}
              src={previewUrl}
              title="Preview"
              className="h-full w-full border-0"
              sandbox="allow-scripts allow-same-origin allow-forms allow-popups allow-modals"
              referrerPolicy="no-referrer"
            />
          ) : previewUrl && !showIframe ? (
            <div className="h-full w-full flex flex-col items-center justify-center text-muted-foreground gap-2">
              <div className="animate-spin rounded-full h-8 w-8 border-b-2 border-primary" />
              <p className="text-sm">Loading preview...</p>
            </div>
          ) : isRunning && showLoadingSpinner ? (
            <div className="h-full w-full flex flex-col items-center justify-center text-muted-foreground gap-2">
              <div className="animate-spin rounded-full h-8 w-8 border-b-2 border-primary" />
              <p className="text-sm">Waiting for preview URL...</p>
            </div>
          ) : isRunning ? (
            <div className="h-full w-full flex flex-col items-center justify-center text-muted-foreground gap-2">
              <p className="text-sm">No preview URL detected</p>
              {allowManualUrl && !detectedUrl && (
                <p className="text-sm text-muted-foreground/70 mt-2 max-w-md text-center">
                  You can manually enter a URL in the input above and press Enter.
                </p>
              )}
            </div>
          ) : (
            <div className="h-full w-full flex items-center justify-center text-muted-foreground">
              Start the dev server to enable preview.
            </div>
          )}
        </SessionPanelContent>


      </div>
    </SessionPanel>
  );
}
