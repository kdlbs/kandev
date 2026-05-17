"use client";

import { memo, useState, useEffect, useMemo, useRef } from "react";
import { IconRefresh, IconExternalLink } from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import { Input } from "@kandev/ui/input";
import { PanelRoot, PanelBody, PanelHeaderBar } from "./panel-primitives";
import { useAppStore } from "@/components/state-provider";
import { detectPreviewUrlFromOutput, rewritePreviewUrlForProxy } from "@/lib/preview-url-detector";
import { InspectButton } from "./inspector/inspect-button";
import { AnnotationsPanel } from "./inspector/annotations-panel";
import { useInspectMode } from "@/hooks/use-inspect-mode";

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
  if (showIframeDelayed) {
    return (
      <iframe
        ref={iframeRef}
        key={refreshKey}
        src={iframeSrc}
        title="Browser Preview"
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

function useBrowserPanelUrl(initialUrl: string, useProxy: boolean) {
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

  const detectedUrl = detectPreviewUrlFromOutput(devOutput);
  const directUrl = useMemo(() => userUrl || detectedUrl || "", [userUrl, detectedUrl]);

  // Proxied variant — only reachable when there is an active session AND the
  // URL is localhost-with-port. Returns null otherwise so the panel can hide
  // the Inspect button.
  const proxiedUrl = useMemo(() => {
    if (!directUrl || !activeSessionId) return null;
    return rewritePreviewUrlForProxy(directUrl, activeSessionId);
  }, [directUrl, activeSessionId]);

  // Default to direct. Caller flips `useProxy` to true (via the Inspect toggle)
  // to route through the agentctl proxy so the inspector script can be injected.
  // Proxying breaks apps that rely on root-absolute asset URLs until URL
  // rewriting lands (tracked separately), so we keep it opt-in.
  const iframeSrc = useProxy && proxiedUrl ? proxiedUrl : directUrl;

  useEffect(() => {
    if (!iframeSrc) return;
    const showTimer = setTimeout(() => setShowIframe(true), 1500);
    return () => {
      setShowIframe(false);
      clearTimeout(showTimer);
    };
  }, [iframeSrc, refreshKey]);

  const displayDraft = urlDraft || detectedUrl || "";
  const showIframeDelayed: string | false = showIframe ? iframeSrc : false;

  function handleUrlSubmit() {
    const trimmed = urlDraft.trim();
    if (trimmed) setUserUrl(trimmed);
  }

  function handleOpenInTab() {
    if (directUrl) window.open(directUrl, "_blank", "noopener,noreferrer");
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
  const iframeRef = useRef<HTMLIFrameElement>(null);
  const inspect = useInspectMode(iframeRef);
  // Inspect mode = "load this page through the proxy so the inspector script
  // can be injected". Toggling Inspect remounts the iframe with a different src.
  const url = useBrowserPanelUrl((params.url as string) || "", inspect.isInspectMode);
  const showInspect = url.canProxy;

  return (
    <PanelRoot>
      <PanelHeaderBar>
        <Input
          value={url.displayDraft}
          onChange={(e) => url.setUrlDraft(e.target.value)}
          onKeyDown={(e) => {
            if (e.key === "Enter") {
              e.preventDefault();
              url.handleUrlSubmit();
            }
          }}
          placeholder={url.detectedUrl || "http://localhost:3000"}
          className="h-6 flex-1 min-w-[180px]"
        />
        <Button
          size="sm"
          variant="outline"
          onClick={url.handleOpenInTab}
          disabled={!url.directUrl}
          className="cursor-pointer"
          title="Open in browser tab"
        >
          <IconExternalLink className="h-4 w-4" />
        </Button>
        <Button
          size="sm"
          variant="outline"
          onClick={() => url.setRefreshKey((v) => v + 1)}
          disabled={!url.directUrl}
          className="cursor-pointer"
          title="Refresh"
        >
          <IconRefresh className="h-4 w-4" />
        </Button>
        {showInspect && (
          <InspectButton
            active={inspect.isInspectMode}
            count={inspect.annotations.length}
            onToggle={inspect.toggleInspect}
          />
        )}
      </PanelHeaderBar>

      <AnnotationsPanel
        annotations={inspect.annotations}
        onRemove={inspect.handleRemoveAnnotation}
        onClear={inspect.handleClearAnnotations}
      />

      <PanelBody padding={false} scroll={false}>
        <BrowserPanelContent
          showIframeDelayed={url.showIframeDelayed}
          iframeSrc={url.iframeSrc}
          refreshKey={url.refreshKey}
          iframeRef={iframeRef}
          onIframeLoad={inspect.handleIframeLoad}
        />
      </PanelBody>
    </PanelRoot>
  );
});
