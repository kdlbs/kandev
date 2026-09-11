"use client";

import { useCallback, useEffect, useRef, useState } from "react";
import { useTheme } from "@/components/theme/app-theme";
import { useResponsiveBreakpoint } from "@/hooks/use-responsive-breakpoint";
import { cn } from "@/lib/utils";
import { useTranslation } from "react-i18next";
import { resolveWebAppAppearance } from "./web-app-appearance";
import {
  createWebAppStartupNonce,
  createWebAppStartupProbe,
  parseWebAppStartupResult,
  WEB_APP_STARTUP_TIMEOUT_MS,
} from "./web-app-startup";

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

type IframeReference = { current: HTMLIFrameElement | null };

type WebAppFrameLifecycleProps = {
  runtimeUrl?: string | null;
  iframeRef: IframeReference;
  sendAppearance: () => void;
  onLoad?: () => void;
  onError?: () => void;
};

function useWebAppFrameLifecycle({
  runtimeUrl,
  iframeRef,
  sendAppearance,
  onLoad,
  onError,
}: WebAppFrameLifecycleProps) {
  const [frameState, setFrameState] = useState<FrameState>(runtimeUrl ? "loading" : "unavailable");
  const frameReadyRef = useRef(false);
  const onLoadRef = useRef(onLoad);
  const onErrorRef = useRef(onError);
  const sendAppearanceRef = useRef(sendAppearance);
  const attemptRef = useRef<{
    nonce: string;
    frameWindow: Window | null;
    settled: boolean;
    timer: ReturnType<typeof setTimeout> | null;
  } | null>(null);

  useEffect(() => {
    onLoadRef.current = onLoad;
    onErrorRef.current = onError;
    sendAppearanceRef.current = sendAppearance;
  }, [onError, onLoad, sendAppearance]);

  useEffect(() => {
    frameReadyRef.current = false;
    setFrameState(runtimeUrl ? "loading" : "unavailable");
    attemptRef.current = null;
  }, [runtimeUrl]);

  useEffect(() => {
    if (frameReadyRef.current) sendAppearance();
  }, [sendAppearance]);

  const finishAttempt = useCallback((result: "ready" | "failed") => {
    const attempt = attemptRef.current;
    if (!attempt || attempt.settled) return;
    attempt.settled = true;
    if (attempt.timer !== null) {
      clearTimeout(attempt.timer);
      attempt.timer = null;
    }
    if (result === "failed") {
      frameReadyRef.current = false;
      setFrameState("unavailable");
      onErrorRef.current?.();
      return;
    }
    frameReadyRef.current = true;
    sendAppearanceRef.current();
    window.requestAnimationFrame(() => {
      if (attemptRef.current !== attempt) return;
      setFrameState("ready");
      onLoadRef.current?.();
    });
  }, []);

  useEffect(() => {
    if (!runtimeUrl) return;
    const attempt = {
      nonce: createWebAppStartupNonce(),
      frameWindow: null as Window | null,
      settled: false,
      timer: null as ReturnType<typeof setTimeout> | null,
    };
    attemptRef.current = attempt;
    attempt.timer = setTimeout(() => finishAttempt("failed"), WEB_APP_STARTUP_TIMEOUT_MS);

    const handleStartupMessage = (event: MessageEvent<unknown>) => {
      if (attemptRef.current !== attempt || attempt.settled) return;
      const currentFrameWindow = iframeRef.current?.contentWindow;
      if (
        !currentFrameWindow ||
        attempt.frameWindow !== currentFrameWindow ||
        event.source !== currentFrameWindow
      ) {
        return;
      }
      const result = parseWebAppStartupResult(event.data, attempt.nonce);
      if (!result) return;
      finishAttempt(result.result);
    };
    window.addEventListener("message", handleStartupMessage);
    return () => {
      window.removeEventListener("message", handleStartupMessage);
      if (attempt.timer !== null) clearTimeout(attempt.timer);
      attempt.timer = null;
      if (attemptRef.current === attempt) attemptRef.current = null;
    };
  }, [finishAttempt, iframeRef, runtimeUrl]);

  const handleLoad = useCallback(() => {
    const attempt = attemptRef.current;
    const frameWindow = iframeRef.current?.contentWindow;
    if (!attempt || attempt.settled || !frameWindow) return;
    attempt.frameWindow = frameWindow;
    sendAppearanceRef.current();
    frameWindow.postMessage(createWebAppStartupProbe(attempt.nonce), "*");
  }, [iframeRef]);

  const handleError = useCallback(() => finishAttempt("failed"), [finishAttempt]);

  return { frameState, handleLoad, handleError };
}

/**
 * Hosts a static plugin web application without bringing it into the SPA
 * process. The sandbox deliberately omits allow-same-origin, top navigation,
 * popups, downloads, and a privileged host bridge.
 */
export function WebAppFrame({ runtimeUrl, title, className, onLoad, onError }: WebAppFrameProps) {
  const { isMobile } = useResponsiveBreakpoint();
  const { t } = useTranslation();
  const { resolvedTheme } = useTheme();
  const iframeRef = useRef<HTMLIFrameElement | null>(null);

  const sendAppearance = useCallback(() => {
    const target = iframeRef.current?.contentWindow;
    if (!target) return;
    target.postMessage(resolveWebAppAppearance(document, resolvedTheme), "*");
  }, [resolvedTheme]);

  const { frameState, handleLoad, handleError } = useWebAppFrameLifecycle({
    runtimeUrl,
    iframeRef,
    sendAppearance,
    onLoad,
    onError,
  });

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
      {runtimeUrl && frameState !== "unavailable" ? (
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
