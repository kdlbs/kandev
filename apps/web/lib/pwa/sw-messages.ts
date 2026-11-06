import { useEffect } from "react";
import { useRouter } from "@/lib/routing/client-router";

/** Mirrors `SW_MESSAGE_NAVIGATE` in `sw/sw-core.ts`; the worker is a separate bundle. */
export const SW_NAVIGATE_MESSAGE = "kandev:navigate";

/**
 * Extracts the in-app path from a worker message, or null when the message is
 * not a navigation request or its target is not an origin-relative path.
 */
export function navigationTargetFromMessage(data: unknown): string | null {
  if (typeof data !== "object" || data === null) return null;
  const { type, url } = data as { type?: unknown; url?: unknown };
  if (type !== SW_NAVIGATE_MESSAGE || typeof url !== "string") return null;
  if (!url.startsWith("/") || url.startsWith("//") || url.includes("\\")) return null;
  return url;
}

/** Routes notification taps (forwarded by the worker) through the SPA router. */
export function useServiceWorkerNavigation(): void {
  const router = useRouter();
  useEffect(() => {
    if (typeof navigator === "undefined" || !("serviceWorker" in navigator)) return;
    const listener = (event: MessageEvent) => {
      const target = navigationTargetFromMessage(event.data);
      if (target) router.push(target);
    };
    navigator.serviceWorker.addEventListener("message", listener);
    return () => navigator.serviceWorker.removeEventListener("message", listener);
  }, [router]);
}
