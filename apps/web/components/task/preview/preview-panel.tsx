"use client";

import { useState, useEffect, useCallback } from "react";
import { IconRefresh, IconExternalLink, IconLoader2 } from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import { Input } from "@kandev/ui/input";
import { SessionPanel, SessionPanelContent } from "@kandev/ui/pannel-session";
import { usePreviewPanel } from "@/hooks/use-preview-panel";
import { useAppStore, useAppStoreApi } from "@/components/state-provider";
import { ShellTerminal } from "@/components/task/shell-terminal";
import { getLocalStorage } from "@/lib/local-storage";
import type { PreviewViewMode, PreviewStage } from "@/lib/state/slices";

type PreviewContentProps = {
  previewView: string;
  previewUrl: string;
  showIframe: boolean;
  refreshKey: number;
  isRunning: boolean;
  showLoadingSpinner: boolean;
  allowManualUrl: boolean;
  detectedUrl: string | null;
  devOutput: string;
  devProcessId: string | undefined;
};

function PreviewContent({
  previewView,
  previewUrl,
  showIframe,
  refreshKey,
  isRunning,
  showLoadingSpinner,
  allowManualUrl,
  detectedUrl,
  devOutput,
  devProcessId,
}: PreviewContentProps) {
  if (previewView === "output") {
    return <ShellTerminal processOutput={devOutput} processId={devProcessId ?? null} />;
  }
  if (showIframe && previewUrl) {
    return (
      <iframe
        key={refreshKey}
        src={previewUrl}
        title="Preview"
        className="h-full w-full border-0"
        sandbox="allow-scripts allow-same-origin allow-forms allow-popups allow-modals"
        referrerPolicy="no-referrer"
      />
    );
  }
  if (previewUrl && !showIframe) {
    return (
      <div className="h-full w-full flex flex-col items-center justify-center text-muted-foreground gap-2">
        <div className="animate-spin rounded-full h-8 w-8 border-b-2 border-primary" />
        <p className="text-sm">Loading preview...</p>
      </div>
    );
  }
  if (isRunning && showLoadingSpinner) {
    return (
      <div className="h-full w-full flex flex-col items-center justify-center text-muted-foreground gap-2">
        <div className="animate-spin rounded-full h-8 w-8 border-b-2 border-primary" />
        <p className="text-sm">Waiting for preview URL...</p>
      </div>
    );
  }
  if (isRunning) {
    return (
      <div className="h-full w-full flex flex-col items-center justify-center text-muted-foreground gap-2">
        <p className="text-sm">No preview URL detected</p>
        {allowManualUrl && !detectedUrl && (
          <p className="text-sm text-muted-foreground/70 mt-2 max-w-md text-center">
            You can manually enter a URL in the input above and press Enter.
          </p>
        )}
      </div>
    );
  }
  return (
    <div className="h-full w-full flex items-center justify-center text-muted-foreground">
      Start the dev server to enable preview.
    </div>
  );
}

type PreviewPanelProps = {
  sessionId: string | null;
  hasDevScript: boolean;
};

function usePreviewViewSync(
  sessionId: string | null,
  setPreviewView: (sid: string, view: PreviewViewMode) => void,
  appStoreApi: ReturnType<typeof useAppStoreApi>,
) {
  useEffect(() => {
    if (!sessionId) return;
    const storeView = appStoreApi.getState().previewPanel.viewBySessionId[sessionId];
    if (storeView) return;
    const key = `preview-view:${sessionId}`;
    const stored = getLocalStorage(key, null as string | null);
    if (stored === "output" || stored === "preview") setPreviewView(sessionId, stored);
  }, [sessionId, appStoreApi, setPreviewView]);
}

type PreviewTimerOptions = {
  isRunning: boolean;
  detectedUrl: string | null;
  sessionId: string | null;
  previewUrl: string;
  refreshKey: number;
  setPreviewView: (sid: string, view: PreviewViewMode) => void;
};

function usePreviewTimers({
  isRunning,
  detectedUrl,
  sessionId,
  previewUrl,
  refreshKey,
  setPreviewView,
}: PreviewTimerOptions) {
  const [allowManualUrl, setAllowManualUrl] = useState(false);
  const [showLoadingSpinner, setShowLoadingSpinner] = useState(true);
  const [showIframe, setShowIframe] = useState(false);

  useEffect(() => {
    if (!isRunning || detectedUrl) {
      const t = setTimeout(() => {
        setAllowManualUrl(false);
        setShowLoadingSpinner(true);
      }, 0);
      return () => clearTimeout(t);
    }
    const resetTimer = setTimeout(() => setShowLoadingSpinner(true), 0);
    const timer = setTimeout(() => {
      setAllowManualUrl(true);
      setShowLoadingSpinner(false);
      if (sessionId) setPreviewView(sessionId, "preview");
    }, 10000);
    return () => {
      clearTimeout(resetTimer);
      clearTimeout(timer);
    };
  }, [isRunning, detectedUrl, sessionId, setPreviewView]);

  useEffect(() => {
    if (!previewUrl) {
      const t = setTimeout(() => setShowIframe(false), 0);
      return () => clearTimeout(t);
    }
    const hideTimer = setTimeout(() => setShowIframe(false), 0);
    const showTimer = setTimeout(() => setShowIframe(true), 2000);
    return () => {
      clearTimeout(hideTimer);
      clearTimeout(showTimer);
    };
  }, [previewUrl, refreshKey]);

  return { allowManualUrl, showLoadingSpinner, showIframe };
}

type PreviewActions = {
  setPreviewOpen: (sid: string, open: boolean) => void;
  setPreviewStage: (sid: string, stage: PreviewStage) => void;
  setPreviewView: (sid: string, view: PreviewViewMode) => void;
  setPreviewUrl: (sid: string, url: string) => void;
  setPreviewUrlDraft: (sid: string, draft: string) => void;
  clearProcessOutput: (pid: string) => void;
  handleStop: () => Promise<void>;
  previewUrl: string;
  previewUrlDraft: string;
  devProcessId: string | undefined;
};

function usePreviewActions(sessionId: string | null, actions: PreviewActions) {
  const {
    setPreviewOpen,
    setPreviewStage,
    setPreviewView,
    setPreviewUrl,
    setPreviewUrlDraft,
    clearProcessOutput,
    handleStop,
    previewUrl,
    previewUrlDraft,
    devProcessId,
  } = actions;

  const handleStopClick = useCallback(async () => {
    if (!sessionId) return;
    setPreviewOpen(sessionId, false);
    setPreviewStage(sessionId, "closed");
    setPreviewView(sessionId, "preview");
    setPreviewUrl(sessionId, "");
    setPreviewUrlDraft(sessionId, "");
    if (devProcessId) clearProcessOutput(devProcessId);
    await handleStop();
  }, [
    sessionId,
    setPreviewOpen,
    setPreviewStage,
    setPreviewView,
    setPreviewUrl,
    setPreviewUrlDraft,
    devProcessId,
    clearProcessOutput,
    handleStop,
  ]);

  const handleUrlSubmit = useCallback(() => {
    if (!sessionId) return;
    const trimmed = previewUrlDraft.trim();
    if (trimmed) setPreviewUrl(sessionId, trimmed);
  }, [sessionId, previewUrlDraft, setPreviewUrl]);

  const handleOpenInTab = useCallback(() => {
    if (!sessionId || !previewUrl) return;
    window.open(previewUrl, "_blank", "noopener,noreferrer");
    setPreviewOpen(sessionId, false);
    setPreviewStage(sessionId, "closed");
    setPreviewView(sessionId, "preview");
    setPreviewUrl(sessionId, "");
    setPreviewUrlDraft(sessionId, "");
  }, [
    sessionId,
    previewUrl,
    setPreviewOpen,
    setPreviewStage,
    setPreviewView,
    setPreviewUrl,
    setPreviewUrlDraft,
  ]);

  return { handleStopClick, handleUrlSubmit, handleOpenInTab };
}

function resolveStopLabel(isStopping: boolean, isFailed: boolean, isExited: boolean): string {
  if (isStopping) return "Stopping\u2026";
  if (isFailed || isExited) return "Close";
  return "Stop";
}

type PreviewToolbarProps = {
  sessionId: string;
  previewUrlDraft: string;
  previewUrl: string;
  previewView: string;
  detectedUrl: string | null;
  isStopping: boolean;
  isWaitingForUrl: boolean;
  showLoadingSpinner: boolean;
  stopLabel: string;
  setPreviewUrlDraft: (sid: string, draft: string) => void;
  setPreviewView: (sid: string, view: PreviewViewMode) => void;
  setRefreshKey: React.Dispatch<React.SetStateAction<number>>;
  handleUrlSubmit: () => void;
  handleOpenInTab: () => void;
  handleStopClick: () => void;
};

function PreviewToolbar({
  sessionId,
  previewUrlDraft,
  previewUrl,
  previewView,
  detectedUrl,
  isStopping,
  isWaitingForUrl,
  showLoadingSpinner,
  stopLabel,
  setPreviewUrlDraft,
  setPreviewView,
  setRefreshKey,
  handleUrlSubmit,
  handleOpenInTab,
  handleStopClick,
}: PreviewToolbarProps) {
  return (
    <div className="flex flex-wrap items-center gap-2">
      <Input
        value={previewUrlDraft}
        onChange={(event) => setPreviewUrlDraft(sessionId, event.target.value)}
        onKeyDown={(event) => {
          if (event.key === "Enter") {
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
        onClick={() => setRefreshKey((v) => v + 1)}
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
        {stopLabel}
      </Button>
      <Button
        size="sm"
        variant={previewView === "output" ? "default" : "outline"}
        className="cursor-pointer"
        onClick={() => setPreviewView(sessionId, previewView === "output" ? "preview" : "output")}
      >
        {isWaitingForUrl && showLoadingSpinner && (
          <IconLoader2 className="h-4 w-4 mr-1 animate-spin" />
        )}
        {previewView === "output" ? "Preview" : "Logs"}
      </Button>
    </div>
  );
}

function usePreviewPanelState(sessionId: string | null, hasDevScript: boolean) {
  const panelState = usePreviewPanel({ sessionId, hasDevScript });
  const setPreviewOpen = useAppStore((state) => state.setPreviewOpen);
  const setPreviewStage = useAppStore((state) => state.setPreviewStage);
  const setPreviewView = useAppStore((state) => state.setPreviewView);
  const clearProcessOutput = useAppStore((state) => state.clearProcessOutput);
  const appStoreApi = useAppStoreApi();

  const previewView = useAppStore((state) => {
    if (!sessionId) return "preview";
    const stored = state.previewPanel.viewBySessionId[sessionId];
    if (stored) return stored;
    const key = `preview-view:${sessionId}`;
    const localStored = getLocalStorage(key, null as string | null);
    return localStored === "output" || localStored === "preview" ? localStored : "preview";
  });

  const devProcessId = useAppStore((state) =>
    sessionId ? state.processes.devProcessBySessionId[sessionId] : undefined,
  );
  const devProcess = useAppStore((state) =>
    devProcessId ? state.processes.processesById[devProcessId] : undefined,
  );
  const devOutput = useAppStore((state) =>
    devProcessId ? (state.processes.outputsByProcessId[devProcessId] ?? "") : "",
  );

  return {
    ...panelState,
    setPreviewOpen,
    setPreviewStage,
    setPreviewView,
    clearProcessOutput,
    appStoreApi,
    previewView,
    devProcessId,
    devProcess,
    devOutput,
  };
}

function PreviewPlaceholder({ message }: { message: string }) {
  return (
    <div className="h-full w-full flex items-center justify-center text-muted-foreground mr-[5px]">
      {message}
    </div>
  );
}

export function PreviewPanel({ sessionId, hasDevScript }: PreviewPanelProps) {
  const panelState = usePreviewPanelState(sessionId, hasDevScript);
  const { previewUrl, previewUrlDraft, setPreviewUrl, setPreviewUrlDraft, setPreviewOpen, setPreviewStage, setPreviewView, clearProcessOutput, appStoreApi, previewView, devProcessId, devProcess, devOutput, isStopping, handleStop, detectedUrl, isRunning } = panelState;

  const [refreshKey, setRefreshKey] = useState(0);
  const isWaitingForUrl = isRunning && previewView === "output" && !previewUrl;

  usePreviewViewSync(sessionId, setPreviewView, appStoreApi);
  const { allowManualUrl, showLoadingSpinner, showIframe } = usePreviewTimers({
    isRunning, detectedUrl, sessionId, previewUrl, refreshKey, setPreviewView,
  });
  const { handleStopClick, handleUrlSubmit, handleOpenInTab } = usePreviewActions(sessionId, {
    setPreviewOpen, setPreviewStage, setPreviewView, setPreviewUrl, setPreviewUrlDraft,
    clearProcessOutput, handleStop, previewUrl, previewUrlDraft, devProcessId,
  });

  if (!sessionId) return <PreviewPlaceholder message="Select a session to enable preview." />;
  if (!hasDevScript) return <PreviewPlaceholder message="Configure a dev script to use preview." />;

  const stopLabel = resolveStopLabel(
    isStopping,
    devProcess?.status === "failed",
    devProcess?.status === "exited",
  );

  return (
    <SessionPanel margin="right">
      <div className="h-full flex flex-col gap-2 flex-1">
        <PreviewToolbar
          sessionId={sessionId}
          previewUrlDraft={previewUrlDraft}
          previewUrl={previewUrl}
          previewView={previewView}
          detectedUrl={detectedUrl}
          isStopping={isStopping}
          isWaitingForUrl={isWaitingForUrl}
          showLoadingSpinner={showLoadingSpinner}
          stopLabel={stopLabel}
          setPreviewUrlDraft={setPreviewUrlDraft}
          setPreviewView={setPreviewView}
          setRefreshKey={setRefreshKey}
          handleUrlSubmit={handleUrlSubmit}
          handleOpenInTab={handleOpenInTab}
          handleStopClick={handleStopClick}
        />
        <SessionPanelContent
          className={previewView === "output" || (showIframe && previewUrl) ? "p-0" : ""}
        >
          <PreviewContent
            previewView={previewView}
            previewUrl={previewUrl}
            showIframe={showIframe}
            refreshKey={refreshKey}
            isRunning={isRunning}
            showLoadingSpinner={showLoadingSpinner}
            allowManualUrl={allowManualUrl}
            detectedUrl={detectedUrl}
            devOutput={devOutput}
            devProcessId={devProcessId}
          />
        </SessionPanelContent>
      </div>
    </SessionPanel>
  );
}
