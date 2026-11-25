export const SERVICE_WORKER_URL = "/sw.js";

type RegisterEnv = {
  isProd: boolean;
  webdriver: boolean;
  isSecureContext: boolean;
};

/**
 * Whether this page should register the worker. Production builds only: the
 * dev server does not emit `/sw.js`. Automated browsers (`navigator.webdriver`)
 * opt out so e2e runs keep seeing real network traffic instead of a worker.
 */
export function shouldRegisterServiceWorker(env: RegisterEnv): boolean {
  return env.isProd && !env.webdriver && env.isSecureContext && "serviceWorker" in navigator;
}

/**
 * Registers the service worker after the page has loaded so it never competes
 * with first paint. `updateViaCache: "none"` makes the browser bypass the HTTP
 * cache for `/sw.js` itself, so a deploy is picked up on the next navigation.
 * Registration failures (including an access proxy answering `/sw.js` with a
 * login redirect) are swallowed: the app works without the worker.
 */
export function registerServiceWorker(
  env: RegisterEnv = {
    isProd: import.meta.env.PROD,
    webdriver: navigator.webdriver === true,
    isSecureContext: window.isSecureContext,
  },
): void {
  if (!shouldRegisterServiceWorker(env)) return;
  const register = () => {
    navigator.serviceWorker
      .register(SERVICE_WORKER_URL, { scope: "/", updateViaCache: "none" })
      .catch(() => undefined);
  };
  if (document.readyState === "complete") register();
  else window.addEventListener("load", register, { once: true });
}
