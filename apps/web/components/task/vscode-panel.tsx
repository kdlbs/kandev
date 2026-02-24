"use client";

import { memo, useState, useCallback, useEffect, useRef } from "react";
import { IconBrandVscode, IconLoader2, IconAlertCircle } from "@tabler/icons-react";
import { useTheme } from "next-themes";
import { Button } from "@kandev/ui/button";
import { PanelRoot, PanelBody } from "./panel-primitives";
import { useAppStore } from "@/components/state-provider";
import { startVscode, getVscodeStatus, type VscodeStatus } from "@/lib/api/domains/vscode-api";
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
      <p className="text-sm font-medium">Starting VS Code Server</p>
      {message && <p className="text-xs text-center max-w-xs opacity-70">{message}</p>}
    </div>
  );
}

function VscodeIframe({ url }: { url: string }) {
  const [loaded, setLoaded] = useState(false);

  return (
    <div className="h-full w-full bg-card">
      <iframe
        src={url}
        title="VS Code"
        className={`h-full w-full border-0 transition-opacity duration-200 ${loaded ? "opacity-100" : "opacity-0"}`}
        sandbox="allow-scripts allow-same-origin allow-forms allow-popups allow-modals allow-downloads"
        allow="clipboard-read; clipboard-write"
        referrerPolicy="no-referrer"
        onLoad={() => setTimeout(() => setLoaded(true), 100)}
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
  onRetry,
}: {
  status: VscodeStatus;
  isRunning: boolean;
  isProgress: boolean;
  iframeUrl: string;
  onRetry: () => void;
}) {
  if (isRunning) {
    return <VscodeIframe url={iframeUrl} />;
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

  const iframeUrl = status.status === "running" && status.url ? buildBaseUrl(status.url) : null;

  return {
    status,
    iframeUrl,
    handleRetry,
  };
}

type VscodePanelProps = {
  panelId: string;
};

// eslint-disable-next-line @typescript-eslint/no-unused-vars
export const VscodePanel = memo(function VscodePanel({ panelId }: VscodePanelProps) {
  const { status, iframeUrl, handleRetry } = useVscodeLifecycle();

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
          onRetry={handleRetry}
        />
      </PanelBody>
    </PanelRoot>
  );
});
