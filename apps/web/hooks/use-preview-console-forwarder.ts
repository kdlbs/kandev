"use client";

import { useEffect } from "react";
import {
  isInspectorMessage,
  isPreviewConsoleMessage,
  isPreviewConsoleReadyMessage,
  sendConsoleBind,
} from "@/lib/preview-inspect-bridge";

const PREFIX = "[preview]";

function previewOrigin(iframe: HTMLIFrameElement): string | null {
  try {
    const origin = new URL(iframe.src || window.location.href, window.location.href);
    if (origin.protocol !== "http:" && origin.protocol !== "https:") return null;
    return origin.origin;
  } catch {
    return null;
  }
}

// Pipes iframe `console.log/warn/error/info/debug` calls — forwarded by the
// runtime shim injected by the gateway port-proxy — into the parent window's
// console with a `[preview]` prefix. Lets developers see iframe diagnostics
// without manually switching DevTools' execution context to the iframe.
//
// The `iframeRef` argument is used to verify that incoming messages came from
// the previewed iframe and not from another frame or extension. The origin is
// pinned to the iframe's actual origin, derived from its src. This also lets
// the console bridge bind its target origin when the UI and gateway use
// different ports in dev mode.
export function usePreviewConsoleForwarder(
  iframeRef: React.RefObject<HTMLIFrameElement | null>,
): void {
  useEffect(() => {
    const bindCurrentConsole = () => {
      const iframe = iframeRef.current;
      const origin = iframe ? previewOrigin(iframe) : null;
      if (iframe && origin) sendConsoleBind(iframe, origin);
    };

    function handleMessage(event: MessageEvent) {
      const iframe = iframeRef.current;
      if (!iframe || event.source !== iframe.contentWindow) return;
      const origin = previewOrigin(iframe);
      if (!origin || event.origin !== origin) return;
      if (!isInspectorMessage(event.data)) return;
      if (isPreviewConsoleReadyMessage(event.data)) {
        sendConsoleBind(iframe, event.origin);
        return;
      }
      if (!isPreviewConsoleMessage(event.data)) return;
      const { level, args } = event.data.payload;
      const fn = console[level] ?? console.log;
      fn.call(console, PREFIX, ...args);
    }
    const iframe = iframeRef.current;
    window.addEventListener("message", handleMessage);
    iframe?.addEventListener?.("load", bindCurrentConsole);
    bindCurrentConsole();
    return () => {
      window.removeEventListener("message", handleMessage);
      iframe?.removeEventListener?.("load", bindCurrentConsole);
    };
  }, [iframeRef]);
}
