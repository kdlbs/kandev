import { createTauriInvokeTransport } from "./tauri-event-transport";
import type { DesktopInvokeTransport } from "./updater-adapter";

export type DesktopLinkDisposition = "external" | "webview" | "blocked";
export type ExternalLinkOpenResult = DesktopLinkDisposition | "browser";

type ExternalLinkEnvironment = {
  baseUrl: () => string;
  openWindow: (url: string, target: string, features: string) => Window | null | void;
};

export type DesktopExternalLinkAdapter = {
  isAvailable: () => boolean;
  classify: (url: string) => DesktopLinkDisposition;
  open: (url: string) => Promise<ExternalLinkOpenResult>;
};

function isLoopbackHostname(hostname: string): boolean {
  const normalized = hostname.toLowerCase();
  return (
    normalized === "localhost" ||
    normalized.endsWith(".localhost") ||
    normalized === "[::1]" ||
    normalized === "[::]" ||
    normalized === "0.0.0.0" ||
    normalized.startsWith("[::ffff:7f") ||
    /^127(?:\.\d{1,3}){3}$/.test(normalized)
  );
}

export function classifyDesktopLink(rawUrl: string, baseUrl: string): DesktopLinkDisposition {
  let destination: URL;
  let base: URL;
  try {
    base = new URL(baseUrl);
    destination = new URL(rawUrl, base);
  } catch {
    return "blocked";
  }

  if (destination.username || destination.password) return "blocked";
  if (destination.protocol === "blob:") return "webview";
  if (destination.protocol === "mailto:") return destination.pathname ? "external" : "blocked";
  if (destination.protocol !== "http:" && destination.protocol !== "https:") return "blocked";
  if (destination.origin === base.origin || isLoopbackHostname(destination.hostname)) {
    return "webview";
  }
  return "external";
}

function browserEnvironment(): ExternalLinkEnvironment {
  return {
    baseUrl: () => (typeof window === "undefined" ? "http://localhost/" : window.location.href),
    openWindow: (url, target, features) => window.open(url, target, features),
  };
}

export function createDesktopExternalLinkAdapter(
  transport: DesktopInvokeTransport,
  environment: ExternalLinkEnvironment = browserEnvironment(),
): DesktopExternalLinkAdapter {
  const classify = (url: string) => classifyDesktopLink(url, environment.baseUrl());

  return {
    isAvailable: transport.isAvailable,
    classify,
    async open(url) {
      if (!transport.isAvailable()) {
        environment.openWindow(url, "_blank", "noopener,noreferrer");
        return "browser";
      }

      const disposition = classify(url);
      if (disposition === "external") {
        const resolvedUrl = new URL(url, environment.baseUrl()).href;
        await transport.invoke("open_external_url", { url: resolvedUrl });
      } else if (disposition === "webview") {
        environment.openWindow(url, "_blank", "noopener,noreferrer");
      }
      return disposition;
    },
  };
}

function clickedBlankAnchor(event: MouseEvent): HTMLAnchorElement | null {
  if (event.defaultPrevented || event.button !== 0) return null;
  const target = event.target;
  if (!(target instanceof Element)) return null;
  const anchor = target.closest("a");
  if (!(anchor instanceof HTMLAnchorElement)) return null;
  if (anchor.target.toLowerCase() !== "_blank" || anchor.hasAttribute("download")) return null;
  return anchor;
}

export function subscribeDesktopExternalLinks(
  doc: Document,
  adapter: DesktopExternalLinkAdapter,
): () => void {
  if (!adapter.isAvailable()) return () => undefined;

  const handleClick = (event: MouseEvent) => {
    const anchor = clickedBlankAnchor(event);
    const rawUrl = anchor?.getAttribute("href");
    if (!rawUrl) return;

    const disposition = adapter.classify(rawUrl);
    if (disposition === "webview") return;
    event.preventDefault();
    if (disposition === "external") void adapter.open(rawUrl).catch(() => undefined);
  };
  doc.addEventListener("click", handleClick, true);
  return () => doc.removeEventListener("click", handleClick, true);
}

export const desktopExternalLinks = createDesktopExternalLinkAdapter(createTauriInvokeTransport());

export function openExternalLink(url: string): Promise<ExternalLinkOpenResult> {
  return desktopExternalLinks.open(url);
}
