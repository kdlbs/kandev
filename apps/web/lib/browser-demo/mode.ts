const DEV_DEMO_SESSION_KEY = "kandev-browser-demo:dev-session";

type BrowserDemoEnv = {
  DEV?: boolean;
  VITE_KANDEV_BROWSER_DEMO?: string;
};

type SessionStorage = Pick<Storage, "getItem" | "setItem">;

export function isBrowserDemoDevRouteAvailable(
  env: BrowserDemoEnv = readBrowserDemoEnv(),
): boolean {
  return env.DEV === true;
}

export function shouldInstallBrowserDemo({
  env,
  pathname,
  storage,
  getStorage,
}: {
  env: BrowserDemoEnv;
  pathname: string;
  storage?: SessionStorage;
  getStorage?: () => SessionStorage;
}): boolean {
  if (env.VITE_KANDEV_BROWSER_DEMO === "true") return true;
  if (!isBrowserDemoDevRouteAvailable(env)) return false;
  try {
    storage ??= getStorage?.();
  } catch {
    // Demo entry remains available when the browser denies storage access.
  }

  if (pathname === "/demo") {
    writeDevDemoSession(storage);
    return true;
  }
  return readDevDemoSession(storage);
}

function readBrowserDemoEnv(): BrowserDemoEnv {
  return (import.meta as ImportMeta & { env?: BrowserDemoEnv }).env ?? {};
}

function readDevDemoSession(storage: SessionStorage | undefined): boolean {
  try {
    return storage?.getItem(DEV_DEMO_SESSION_KEY) === "true";
  } catch {
    return false;
  }
}

function writeDevDemoSession(storage: SessionStorage | undefined) {
  try {
    storage?.setItem(DEV_DEMO_SESSION_KEY, "true");
  } catch {
    // The current page can still enter demo mode when storage is unavailable.
  }
}
