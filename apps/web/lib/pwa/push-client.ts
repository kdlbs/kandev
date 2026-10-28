import { ApiError } from "@/lib/api/client";
import { SERVICE_WORKER_URL } from "./register-sw";

const READY_TIMEOUT_MS = 10_000;

export function isPushSupported(): boolean {
  return (
    typeof window !== "undefined" &&
    window.isSecureContext &&
    "serviceWorker" in navigator &&
    "PushManager" in window &&
    typeof Notification !== "undefined"
  );
}

/** Decodes a base64url VAPID key into the bytes `pushManager.subscribe` expects. */
export function urlBase64ToUint8Array(value: string): Uint8Array<ArrayBuffer> {
  const padding = "=".repeat((4 - (value.length % 4)) % 4);
  const base64 = (value + padding).replace(/-/g, "+").replace(/_/g, "/");
  const raw = atob(base64);
  const bytes = new Uint8Array(new ArrayBuffer(raw.length));
  for (let i = 0; i < raw.length; i += 1) bytes[i] = raw.charCodeAt(i);
  return bytes;
}

/**
 * The active worker registration, registering it on demand. Rejects when no
 * worker can be activated (for example a dev server that does not emit one).
 */
export async function getPushRegistration(): Promise<ServiceWorkerRegistration> {
  const existing = await navigator.serviceWorker.getRegistration("/");
  const registration =
    existing ??
    (await navigator.serviceWorker.register(SERVICE_WORKER_URL, {
      scope: "/",
      updateViaCache: "none",
    }));
  if (registration.active) return registration;
  const ready = navigator.serviceWorker.ready;
  const timeout = new Promise<never>((_, reject) =>
    setTimeout(() => reject(new Error("service worker not ready")), READY_TIMEOUT_MS),
  );
  return Promise.race([ready, timeout]);
}

/**
 * True when an API failure means the login/session is gone rather than the
 * feature being broken: a 401/403, or a network-level failure while the device
 * is online, which is what a cross-origin redirect to an access proxy's login
 * page looks like to `fetch`.
 */
export function isSessionExpiredError(error: unknown): boolean {
  if (error instanceof ApiError) return error.status === 401 || error.status === 403;
  return error instanceof TypeError && navigator.onLine;
}
