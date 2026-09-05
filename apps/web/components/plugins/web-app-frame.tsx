"use client";

import { useCallback, useEffect, useRef, useState } from "react";
import { useTheme } from "@/components/theme/app-theme";
import { useResponsiveBreakpoint } from "@/hooks/use-responsive-breakpoint";
import { cn } from "@/lib/utils";
import { useTranslation } from "react-i18next";
import { resolveWebAppAppearance } from "./web-app-appearance";

type WebAppFrameProps = {
  /** A short-lived capability URL returned by the backend. */
  runtimeUrl?: string | null;
  /** The host-owned accessible name. Runtime content is untrusted. */
  title: string;
  className?: string;
  onLoad?: () => void;
  onError?: () => void;
};

type FrameState = "loading" | "ready" | "unavailable";

/**
 * Hosts a static plugin web application without bringing it into the SPA
 * process. The sandbox deliberately omits allow-same-origin, top navigation,
 * popups, downloads, and a privileged host bridge.
 */
export function WebAppFrame({ runtimeUrl, title, className, onLoad, onError }: WebAppFrameProps) {
  const { isMobile } = useResponsiveBreakpoint();
  const { t } = useTranslation();
  const { resolvedTheme } = useTheme();
  const [frameState, setFrameState] = useState<FrameState>(runtimeUrl ? "loading" : "unavailable");
  const iframeRef = useRef<HTMLIFrameElement | null>(null);
  const frameReadyRef = useRef(false);

  useEffect(() => {
    frameReadyRef.current = false;
    setFrameState(runtimeUrl ? "loading" : "unavailable");
  }, [runtimeUrl]);

  const sendAppearance = useCallback(() => {
    const target = iframeRef.current?.contentWindow;
    if (!target) return;
    target.postMessage(resolveWebAppAppearance(document, resolvedTheme), "*");
  }, [resolvedTheme]);

  useEffect(() => {
    if (frameReadyRef.current) sendAppearance();
  }, [sendAppearance]);

  const handleLoad = () => {
    sendAppearance();
    frameReadyRef.current = true;
    onLoad?.();
    window.requestAnimationFrame(() => setFrameState("ready"));
  };

  const handleError = () => {
    frameReadyRef.current = false;
    setFrameState("unavailable");
    onError?.();
  };

  return (
    <div
      data-testid="web-app-frame"
      data-frame-state={frameState}
      data-mobile={isMobile ? "true" : "false"}
      className={cn(
        "relative flex min-h-0 min-w-0 flex-1 overflow-hidden",
        isMobile && "pb-[env(safe-area-inset-bottom)]",
        className,
      )}
      aria-busy={frameState === "loading"}
    >
      {runtimeUrl ? (
        <iframe
          key={runtimeUrl}
          title={title}
          src={runtimeUrl}
          ref={iframeRef}
          sandbox="allow-scripts allow-forms"
          referrerPolicy="no-referrer"
          loading="eager"
          className="block h-full min-h-0 w-full min-w-0 flex-1 border-0"
          onLoad={handleLoad}
          onError={handleError}
        />
      ) : null}
      {frameState !== "ready" && (
        <div
          role={frameState === "unavailable" ? "alert" : "status"}
          aria-live="polite"
          className="pointer-events-none absolute inset-0 flex items-center justify-center bg-background/80 p-4 text-center text-sm text-muted-foreground"
        >
          {t(frameState === "unavailable" ? "plugins:webAppUnavailable" : "plugins:webAppLoading")}
        </div>
      )}
    </div>
  );
}
