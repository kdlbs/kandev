"use client";

import { memo, useState, useEffect, useMemo, useRef } from "react";
import { IconRefresh, IconExternalLink } from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import { Input } from "@kandev/ui/input";
import { PanelRoot, PanelBody, PanelHeaderBar } from "./panel-primitives";
import { useAppStore } from "@/components/state-provider";
import { detectPreviewUrlFromOutput, rewritePreviewUrlForProxy } from "@/lib/preview-url-detector";
import { PreviewFeedbackControls } from "./inspector/preview-feedback-controls";
import { usePreviewCapture } from "@/hooks/use-preview-capture";
import { usePreviewConsoleForwarder } from "@/hooks/use-preview-console-forwarder";
import { openExternalLink } from "@/lib/desktop/external-links";
import { useTranslation } from "react-i18next";

function BrowserPanelContent({
  showIframeDelayed,
  iframeSrc,
  refreshKey,
  iframeRef,
  onIframeLoad,
}: {
  showIframeDelayed: string | false;
  iframeSrc: string;
  refreshKey: number;
  iframeRef: React.RefObject<HTMLIFrameElement | null>;
  onIframeLoad: () => void;
}) {
  const { t } = useTranslation();
  if (showIframeDelayed) {
    return (
      <iframe
        ref={iframeRef}
        key={refreshKey}
        src={iframeSrc}
        title={t("task:browserPreview")}
        className="h-full w-full border-0"
        sandbox="allow-scripts allow-same-origin allow-forms allow-popups allow-modals"
        referrerPolicy="no-referrer"
        onLoad={onIframeLoad}
      />
    );
  }
  if (iframeSrc) {
    return (
      <div className="h-full w-full flex flex-col items-center justify-center text-muted-foreground gap-2">
        <div className="animate-spin rounded-full h-8 w-8 border-b-2 border-primary" />
        <p className="text-sm">{t("task:loadingPreview")}</p>
      </div>
    );
  }
  return (
    <div className="h-full w-full flex flex-col items-center justify-center text-muted-foreground gap-2">
      <p className="text-sm">{t("task:enterAUrlAboveOrStart")}</p>
    </div>
  );
}

type BrowserPanelProps = {
  panelId: string;
  params: Record<string, unknown>;
};

function useBrowserPanelUrl(initialUrl: string) {
  const [userUrl, setUserUrl] = useState(initialUrl);
  const [urlDraft, setUrlDraft] = useState(initialUrl);
  const [refreshKey, setRefreshKey] = useState(0);
  const [showIframe, setShowIframe] = useState(false);

  useEffect(() => {
    setUserUrl(initialUrl);
    setUrlDraft(initialUrl);
  }, [initialUrl]);

  const activeSessionId = useAppStore((state) => state.tasks.activeSessionId);
  const devProcessId = useAppStore((state) =>
    activeSessionId ? state.processes.devProcessBySessionId[activeSessionId] : undefined,
  );
  const devOutput = useAppStore((state) =>
    devProcessId ? (state.processes.outputsByProcessId[devProcessId] ?? "") : "",
  );

  const detectedUrl = detectPreviewUrlFromOutput(devOutput);
  const directUrl = useMemo(() => userUrl || detectedUrl || "", [userUrl, detectedUrl]);

  // Proxied variant — only reachable when there is an active session AND the
  // URL is localhost-with-port. Returns null otherwise so the panel can hide
  // the Inspect button.
  const proxiedUrl = useMemo(() => {
    if (!directUrl || !activeSessionId) return null;
    return rewritePreviewUrlForProxy(directUrl, activeSessionId);
  }, [directUrl, activeSessionId]);

  // Eligible local pages stay behind the proxy so task markers and the capture
  // bridge survive route changes even while selection mode is inactive.
  const iframeSrc = proxiedUrl ?? directUrl;

  // Key the loading-spinner gate to the underlying URL (and the refresh key),
  // NOT to `iframeSrc`. Toggling Inspect mode flips `iframeSrc` between the
  // direct URL and the proxied URL even though the user's destination didn't
  // change; if the effect re-ran on every `iframeSrc` change, the cleanup
  // would hide the iframe and show the 1.5s spinner on every Inspect toggle.
  // Browser-level iframe navigation handles direct↔proxy swaps fine.
  useEffect(() => {
    if (!directUrl) return;
    const showTimer = setTimeout(() => setShowIframe(true), 1500);
    return () => {
      setShowIframe(false);
      clearTimeout(showTimer);
    };
  }, [directUrl, refreshKey]);

  const displayDraft = urlDraft || detectedUrl || "";
  const showIframeDelayed: string | false = showIframe ? iframeSrc : false;

  function handleUrlSubmit() {
    const trimmed = urlDraft.trim();
    if (trimmed) setUserUrl(trimmed);
  }

  function handleOpenInTab() {
    if (directUrl) void openExternalLink(directUrl).catch(() => undefined);
  }

  return {
    directUrl,
    iframeSrc,
    canProxy: !!proxiedUrl,
    refreshKey,
    setRefreshKey,
    urlDraft,
    setUrlDraft,
    displayDraft,
    detectedUrl,
    showIframeDelayed,
    handleUrlSubmit,
    handleOpenInTab,
  };
}

export const BrowserPanel = memo(function BrowserPanel({ params }: BrowserPanelProps) {
  const { t } = useTranslation();
  const iframeRef = useRef<HTMLIFrameElement>(null);
  usePreviewConsoleForwarder(iframeRef);
  const activeTaskId = useAppStore((state) => state.tasks.activeTaskId);
  const activeSessionId = useAppStore((state) => state.tasks.activeSessionId);
  const url = useBrowserPanelUrl((params.url as string) || "");
  const showInspect = url.canProxy;
  const capture = usePreviewCapture({
    taskId: activeTaskId,
    iframeRef,
    enabled: showInspect && !!activeTaskId,
    source: {
      kind: "browser",
      sessionId: activeSessionId ?? undefined,
      label: url.directUrl,
    },
  });

  return (
    <PanelRoot data-testid="browser-panel">
      <PanelHeaderBar className="h-auto min-h-[52px] py-1 sm:h-[30px] sm:min-h-[30px] sm:py-0">
        <Input
          controlSize="none"
          value={url.displayDraft}
          onChange={(e) => url.setUrlDraft(e.target.value)}
          onKeyDown={(e) => {
            if (e.key === "Enter") {
              e.preventDefault();
              url.handleUrlSubmit();
            }
          }}
          placeholder={url.detectedUrl || "http://localhost:3000"}
          className="h-11 flex-1 min-w-0 sm:h-6 sm:min-w-[180px]"
        />
        <Button
          size="sm"
          variant="outline"
          onClick={url.handleOpenInTab}
          disabled={!url.directUrl}
          className="h-11 w-11 cursor-pointer p-0 sm:h-6 sm:w-auto sm:px-2"
          title={t("task:openInBrowserTab")}
        >
          <IconExternalLink className="h-4 w-4" />
        </Button>
        <Button
          size="sm"
          variant="outline"
          onClick={() => url.setRefreshKey((v) => v + 1)}
          disabled={!url.directUrl}
          className="h-11 w-11 cursor-pointer p-0 sm:h-6 sm:w-auto sm:px-2"
          title={t("task:refresh")}
        >
          <IconRefresh className="h-4 w-4" />
        </Button>
        {showInspect && <PreviewFeedbackControls capture={capture} enabled={!!activeTaskId} />}
      </PanelHeaderBar>

      <PanelBody padding={false} scroll={false}>
        <BrowserPanelContent
          showIframeDelayed={url.showIframeDelayed}
          iframeSrc={url.iframeSrc}
          refreshKey={url.refreshKey}
          iframeRef={iframeRef}
          onIframeLoad={capture.handleIframeLoad}
        />
      </PanelBody>
    </PanelRoot>
  );
});
