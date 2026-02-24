"use client";

import { memo, useState, useEffect, useMemo } from "react";
import { IconRefresh, IconExternalLink } from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import { Input } from "@kandev/ui/input";
import { PanelRoot, PanelBody, PanelHeaderBar } from "./panel-primitives";
import { useAppStore } from "@/components/state-provider";
import { detectPreviewUrlFromOutput } from "@/lib/preview-url-detector";

function BrowserPanelContent({
  showIframeDelayed,
  effectiveUrl,
  refreshKey,
}: {
  showIframeDelayed: string | false;
  effectiveUrl: string;
  refreshKey: number;
}) {
  if (showIframeDelayed) {
    return (
      <iframe
        key={refreshKey}
        src={effectiveUrl}
        title="Browser Preview"
        className="h-full w-full border-0"
        sandbox="allow-scripts allow-same-origin allow-forms allow-popups allow-modals"
        referrerPolicy="no-referrer"
      />
    );
  }
  if (effectiveUrl) {
    return (
      <div className="h-full w-full flex flex-col items-center justify-center text-muted-foreground gap-2">
        <div className="animate-spin rounded-full h-8 w-8 border-b-2 border-primary" />
        <p className="text-sm">Loading preview...</p>
      </div>
    );
  }
  return (
    <div className="h-full w-full flex flex-col items-center justify-center text-muted-foreground gap-2">
      <p className="text-sm">Enter a URL above or start the dev server</p>
    </div>
  );
}

type BrowserPanelProps = {
  panelId: string;
  params: Record<string, unknown>;
};

export const BrowserPanel = memo(function BrowserPanel({ params }: BrowserPanelProps) {
  const initialUrl = (params.url as string) || "";
  const [userUrl, setUserUrl] = useState(initialUrl);
  const [urlDraft, setUrlDraft] = useState(initialUrl);
  const [refreshKey, setRefreshKey] = useState(0);
  const [showIframe, setShowIframe] = useState(false);

  const activeSessionId = useAppStore((state) => state.tasks.activeSessionId);
  const devProcessId = useAppStore((state) =>
    activeSessionId ? state.processes.devProcessBySessionId[activeSessionId] : undefined,
  );
  const devOutput = useAppStore((state) =>
    devProcessId ? (state.processes.outputsByProcessId[devProcessId] ?? "") : "",
  );

  // Auto-detect URL from dev server output
  const detectedUrl = detectPreviewUrlFromOutput(devOutput);
  // Use user-set URL if available, otherwise fall back to detected URL
  const effectiveUrl = useMemo(() => userUrl || detectedUrl || "", [userUrl, detectedUrl]);

  // Derive iframe visibility: show after a delay when URL is available
  const showIframeDelayed: string | false = showIframe ? effectiveUrl : false;

  // Delay showing iframe after URL is set
  useEffect(() => {
    if (!effectiveUrl) return;
    const showTimer = setTimeout(() => setShowIframe(true), 1500);
    return () => {
      setShowIframe(false);
      clearTimeout(showTimer);
    };
  }, [effectiveUrl, refreshKey]);

  // Sync draft with detected URL when user hasn't typed anything
  const displayDraft = urlDraft || detectedUrl || "";

  const handleUrlSubmit = () => {
    const trimmed = urlDraft.trim();
    if (trimmed) {
      setUserUrl(trimmed);
    }
  };

  const handleOpenInTab = () => {
    if (effectiveUrl) {
      window.open(effectiveUrl, "_blank", "noopener,noreferrer");
    }
  };

  return (
    <PanelRoot>
      <PanelHeaderBar>
        <Input
          value={displayDraft}
          onChange={(e) => setUrlDraft(e.target.value)}
          onKeyDown={(e) => {
            if (e.key === "Enter") {
              e.preventDefault();
              handleUrlSubmit();
            }
          }}
          placeholder={detectedUrl || "http://localhost:3000"}
          className="h-6 flex-1 min-w-[180px]"
        />
        <Button
          size="sm"
          variant="outline"
          onClick={handleOpenInTab}
          disabled={!effectiveUrl}
          className="cursor-pointer"
          title="Open in browser tab"
        >
          <IconExternalLink className="h-4 w-4" />
        </Button>
        <Button
          size="sm"
          variant="outline"
          onClick={() => setRefreshKey((v) => v + 1)}
          disabled={!effectiveUrl}
          className="cursor-pointer"
          title="Refresh"
        >
          <IconRefresh className="h-4 w-4" />
        </Button>
      </PanelHeaderBar>

      <PanelBody padding={false} scroll={false}>
        <BrowserPanelContent
          showIframeDelayed={showIframeDelayed}
          effectiveUrl={effectiveUrl}
          refreshKey={refreshKey}
        />
      </PanelBody>
    </PanelRoot>
  );
});
