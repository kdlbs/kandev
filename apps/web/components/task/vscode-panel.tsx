"use client";

import { memo, useState, useCallback, useEffect, useRef } from "react";
import type { IDockviewPanelProps } from "dockview-react";
import {
  IconRefresh,
  IconExternalLink,
  IconPlayerStop,
  IconBrandVscode,
  IconLoader2,
  IconAlertCircle,
} from "@tabler/icons-react";
import { useTheme } from "next-themes";
import { Button } from "@kandev/ui/button";
import { PanelRoot, PanelBody, PanelFooterBar } from "./panel-primitives";
import { useAppStore } from "@/components/state-provider";
import { useDockviewStore } from "@/lib/state/dockview-store";
import {
  startVscode,
  stopVscode,
  getVscodeStatus,
  type VscodeStatus,
} from "@/lib/api/domains/vscode-api";
import { getBackendConfig } from "@/lib/config";

function VscodeProgress({
  status,
  message,
  error,
  onRetry,
}: {
  status: VscodeStatus["status"];
  message?: string;
  error?: string;
  onRetry: () => void;
}) {
  if (status === "error") {
    return (
      <div className="h-full w-full flex flex-col items-center justify-center text-muted-foreground gap-4">
        <IconAlertCircle className="h-12 w-12 text-destructive opacity-70" />
        <p className="text-sm font-medium text-destructive">Failed to start VS Code</p>
        {error && <p className="text-xs text-destructive/80 text-center max-w-xs">{error}</p>}
        <Button size="sm" onClick={onRetry} className="cursor-pointer">
          Retry
        </Button>
      </div>
    );
  }

  return (
    <div className="h-full w-full flex flex-col items-center justify-center text-muted-foreground gap-4">
      <IconLoader2 className="h-10 w-10 text-blue-500 animate-spin" />
      <p className="text-sm font-medium">
        {status === "installing" ? "Installing VS Code Server" : "Starting VS Code Server"}
      </p>
      {message && <p className="text-xs text-center max-w-xs opacity-70">{message}</p>}
    </div>
  );
}

function VscodeIframe({ url }: { url: string }) {
  const [loaded, setLoaded] = useState(false);

  const handleLoad = useCallback(() => {
    setTimeout(() => setLoaded(true), 300);
  }, []);

  return (
    <div className="relative h-full w-full bg-card">
      {!loaded && (
        <div className="absolute inset-0 flex items-center justify-center">
          <IconLoader2 className="h-8 w-8 text-muted-foreground animate-spin" />
        </div>
      )}
      <iframe
        src={url}
        title="VS Code"
        className={`h-full w-full border-0 ${loaded ? "opacity-100" : "opacity-0"}`}
        sandbox="allow-scripts allow-same-origin allow-forms allow-popups allow-modals allow-downloads"
        allow="clipboard-read; clipboard-write"
        referrerPolicy="no-referrer"
        onLoad={handleLoad}
      />
    </div>
  );
}

function VscodeIdle() {
  return (
    <div className="h-full w-full flex flex-col items-center justify-center text-muted-foreground gap-4">
      <IconBrandVscode className="h-12 w-12 text-blue-500 opacity-50" />
      <p className="text-sm font-medium">VS Code Server</p>
      <p className="text-xs text-center max-w-xs opacity-70">Waiting for an active session...</p>
    </div>
  );
}

function VscodePanelBody({
  status,
  isRunning,
  isProgress,
  iframeUrl,
  refreshKey,
  onRetry,
}: {
  status: VscodeStatus;
  isRunning: boolean;
  isProgress: boolean;
  iframeUrl: string;
  refreshKey: number;
  onRetry: () => void;
}) {
  if (isRunning) {
    return <VscodeIframe key={refreshKey} url={iframeUrl} />;
  }
  if (isProgress) {
    return (
      <VscodeProgress
        status={status.status}
        message={status.message}
        error={status.error}
        onRetry={onRetry}
      />
    );
  }
  return <VscodeIdle />;
}

/** Build the base iframe URL from the VS Code proxy path. */
function buildBaseUrl(proxyPath: string): string {
  const { apiBaseUrl } = getBackendConfig();
  return `${apiBaseUrl}${proxyPath}`;
}

/** Manages auto-start, polling, and navigation for the VS Code panel. */
function useVscodeLifecycle() {
  const activeSessionId = useAppStore((state) => state.tasks.activeSessionId);
  const { resolvedTheme } = useTheme();
  const [status, setStatus] = useState<VscodeStatus>({ status: "stopped" });
  const [refreshKey, setRefreshKey] = useState(0);
  const startedRef = useRef(false);

  // Auto-start on mount or session change
  useEffect(() => {
    if (!activeSessionId) return;
    startedRef.current = false;

    let cancelled = false;
    getVscodeStatus(activeSessionId)
      .then((s) => {
        if (cancelled) return;
        setStatus(s);

        if (s.status === "stopped" && !startedRef.current) {
          startedRef.current = true;
          const theme = resolvedTheme === "light" ? "light" : "dark";
          startVscode(activeSessionId, theme)
            .then((result) => {
              if (!cancelled) setStatus(result);
            })
            .catch(() => {
              if (!cancelled) setStatus({ status: "error", error: "Failed to start VS Code" });
            });
        }
      })
      .catch(() => {
        if (!cancelled) setStatus({ status: "error", error: "Failed to get VS Code status" });
      });

    return () => {
      cancelled = true;
    };
  }, [activeSessionId, resolvedTheme]);

  // Poll for status while installing or starting
  useEffect(() => {
    if (!activeSessionId) return;
    if (status.status !== "installing" && status.status !== "starting") return;

    const interval = setInterval(() => {
      getVscodeStatus(activeSessionId).then(setStatus);
    }, 2000);

    return () => clearInterval(interval);
  }, [activeSessionId, status.status]);

  const handleStop = useCallback(async () => {
    if (!activeSessionId) return;
    await stopVscode(activeSessionId);
    setStatus({ status: "stopped" });
    startedRef.current = false;
    // Remove the vscode panel from the current layout instead of switching layouts
    const dockApi = useDockviewStore.getState().api;
    const vscodePanel = dockApi?.getPanel("vscode");
    if (vscodePanel) {
      try {
        dockApi!.removePanel(vscodePanel);
      } catch {
        /* already removed */
      }
    }
  }, [activeSessionId]);

  const handleRetry = useCallback(() => {
    if (!activeSessionId) return;
    startedRef.current = true;
    const theme = resolvedTheme === "light" ? "light" : "dark";
    setStatus({ status: "installing", message: "Retrying..." });
    startVscode(activeSessionId, theme)
      .then(setStatus)
      .catch(() => {
        setStatus({ status: "error", error: "Failed to start VS Code" });
      });
  }, [activeSessionId, resolvedTheme]);

  const handleRefresh = useCallback(() => {
    setRefreshKey((k) => k + 1);
  }, []);

  const handleOpenInTab = useCallback(() => {
    if (status.url) {
      window.open(buildBaseUrl(status.url), "_blank", "noopener,noreferrer");
    }
  }, [status.url]);

  const iframeUrl = status.status === "running" && status.url ? buildBaseUrl(status.url) : null;

  return {
    status,
    iframeUrl,
    refreshKey,
    handleStop,
    handleRetry,
    handleRefresh,
    handleOpenInTab,
  };
}

export const VscodePanel = memo(function VscodePanel(
  _props: IDockviewPanelProps, // eslint-disable-line @typescript-eslint/no-unused-vars
) {
  const { status, iframeUrl, refreshKey, handleStop, handleRetry, handleRefresh, handleOpenInTab } =
    useVscodeLifecycle();

  const isRunning = status.status === "running" && iframeUrl;
  const isProgress =
    status.status === "installing" || status.status === "starting" || status.status === "error";

  return (
    <PanelRoot>
      <PanelBody padding={false} scroll={false}>
        <VscodePanelBody
          status={status}
          isRunning={!!isRunning}
          isProgress={isProgress}
          iframeUrl={iframeUrl ?? ""}
          refreshKey={refreshKey}
          onRetry={handleRetry}
        />
      </PanelBody>
      <VscodePanelFooter
        status={status.status}
        isRunning={!!isRunning}
        onOpenInTab={handleOpenInTab}
        onRefresh={handleRefresh}
        onStop={handleStop}
      />
    </PanelRoot>
  );
});

function VscodePanelFooter({
  status,
  isRunning,
  onOpenInTab,
  onRefresh,
  onStop,
}: {
  status: VscodeStatus["status"];
  isRunning: boolean;
  onOpenInTab: () => void;
  onRefresh: () => void;
  onStop: () => void;
}) {
  return (
    <PanelFooterBar>
      <div className="flex items-center gap-1 text-xs text-muted-foreground px-1">
        <IconBrandVscode className="h-3.5 w-3.5" />
        <span>VS Code</span>
        {isRunning && <span className="ml-1 h-1.5 w-1.5 rounded-full bg-green-500" />}
        {(status === "installing" || status === "starting") && (
          <IconLoader2 className="ml-1 h-3 w-3 animate-spin text-blue-500" />
        )}
      </div>
      <div className="flex-1" />
      {isRunning && (
        <>
          <Button
            size="sm"
            variant="outline"
            onClick={onOpenInTab}
            className="cursor-pointer"
            title="Open in browser tab"
          >
            <IconExternalLink className="h-4 w-4" />
          </Button>
          <Button
            size="sm"
            variant="outline"
            onClick={onRefresh}
            className="cursor-pointer"
            title="Refresh"
          >
            <IconRefresh className="h-4 w-4" />
          </Button>
          <Button
            size="sm"
            variant="outline"
            onClick={onStop}
            className="cursor-pointer"
            title="Stop VS Code"
          >
            <IconPlayerStop className="h-4 w-4" />
          </Button>
        </>
      )}
    </PanelFooterBar>
  );
}
