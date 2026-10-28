/**
 * Kandev service worker.
 *
 * Scope is deliberately narrow: it precaches a handful of static shell files,
 * answers navigations network-first with an offline page, caches content-hashed
 * build assets, and handles Web Push. It never sees API or WebSocket traffic
 * (see `routeRequest`) and never stores a navigation response, because the
 * SPA shell carries a per-request boot payload.
 */
import {
  SW_MESSAGE_NAVIGATE,
  cacheNames,
  isCacheableAsset,
  isCacheablePrecache,
  notificationOptions,
  parsePushPayload,
  pickClientForUrl,
  routeRequest,
  sanitizeTargetUrl,
  shouldSuppressNotification,
  staleCacheNames,
} from "./sw-core";
import type { ServiceWorkerScope } from "./sw-types";

declare const __KANDEV_SW_BUILD_ID__: string;
declare const __KANDEV_SW_PRECACHE__: string[];

const sw = self as unknown as ServiceWorkerScope;
const BUILD_ID = __KANDEV_SW_BUILD_ID__;
const OFFLINE_URL = "/offline.html";
const ICON_URL = "/web-app-manifest-192x192.png";
const FALLBACK_TITLE = "Kandev"; // i18n-exempt: product name
const PRECACHE = new Set<string>([OFFLINE_URL, ...__KANDEV_SW_PRECACHE__]);
const names = cacheNames(BUILD_ID);

sw.addEventListener("install", (event) => {
  event.waitUntil(
    (async () => {
      const cache = await sw.caches.open(names.shell);
      // Entries are fetched one by one so a single failure (for example an
      // access-proxy redirect during install) does not abort the whole worker;
      // anything missing is simply fetched from the network later.
      await Promise.all(
        [...PRECACHE].map(async (path) => {
          try {
            const response = await fetch(path, { credentials: "same-origin", cache: "reload" });
            if (isCacheablePrecache(response)) await cache.put(path, response);
          } catch {
            // Offline or blocked during install: fall back to the network.
          }
        }),
      );
      await sw.skipWaiting();
    })(),
  );
});

sw.addEventListener("activate", (event) => {
  event.waitUntil(
    (async () => {
      const existing = await sw.caches.keys();
      await Promise.all(staleCacheNames(existing, BUILD_ID).map((name) => sw.caches.delete(name)));
      await sw.clients.claim();
    })(),
  );
});

sw.addEventListener("fetch", (event) => {
  const route = routeRequest(event.request, sw.location.origin, PRECACHE);
  if (route === "navigate") {
    event.respondWith(handleNavigation(event.request));
  } else if (route === "asset") {
    event.respondWith(handleAsset(event.request));
  } else if (route === "precache") {
    event.respondWith(handlePrecached(event.request));
  }
});

/**
 * Network-first. Whatever the network returns, including an opaque redirect to
 * a login page, is passed straight to the browser and never stored. Only a
 * failed fetch (offline) falls back to the cached offline page.
 */
async function handleNavigation(request: Request): Promise<Response> {
  try {
    return await fetch(request);
  } catch {
    const offline = await sw.caches.match(OFFLINE_URL);
    return offline ?? Response.error();
  }
}

/**
 * Hashed build assets are immutable, so cache-first is safe once stored. The
 * lookup spans both caches so precached shell files are served from the shell
 * cache; new files are stored in the assets cache.
 */
async function handleAsset(request: Request): Promise<Response> {
  const hit = await sw.caches.match(request);
  if (hit) return hit;
  const response = await fetch(request);
  if (isCacheableAsset(response)) {
    const cache = await sw.caches.open(names.assets);
    void cache.put(request, response.clone());
  }
  return response;
}

async function handlePrecached(request: Request): Promise<Response> {
  const hit = await sw.caches.match(request);
  if (hit) return hit;
  return fetch(request);
}

sw.addEventListener("push", (event) => {
  const raw = event.data ? event.data.text() : null;
  const payload = parsePushPayload(raw, FALLBACK_TITLE);
  event.waitUntil(
    (async () => {
      const windows = await sw.clients.matchAll({ type: "window", includeUncontrolled: true });
      if (shouldSuppressNotification(windows)) return;
      await sw.registration.showNotification(payload.title, notificationOptions(payload, ICON_URL));
    })(),
  );
});

sw.addEventListener("notificationclick", (event) => {
  event.notification.close();
  const data = (event.notification.data ?? {}) as { url?: string };
  const target = sanitizeTargetUrl(data.url ?? "/");
  event.waitUntil(
    (async () => {
      const windows = await sw.clients.matchAll({ type: "window", includeUncontrolled: true });
      const client = pickClientForUrl(windows, target, sw.location.origin);
      if (client) {
        await client.focus();
        client.postMessage({ type: SW_MESSAGE_NAVIGATE, url: target });
        return;
      }
      await sw.clients.openWindow(target);
    })(),
  );
});
