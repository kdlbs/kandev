import { StrictMode, useEffect } from "react";
import { createRoot } from "react-dom/client";
import "@/app/globals.css";
import { setOnUnauthorized } from "@/lib/api/client";
import { PluginModalHost } from "@/components/plugins/plugin-modal-host";
import { useAppStoreApi, StateProvider } from "@/components/state-provider";
import { PluginBootBridge } from "@/lib/plugins/plugin-boot-bridge";
import { preloadLocale } from "@/lib/i18n";
import { resolveInitialLocale } from "@/lib/i18n/boot";
import { AppShell } from "./app-shell";
import { AuthGatedScreen, useAuthGateDecision } from "./auth-gate";
import { loadBootPayload } from "./boot-payload";
import type { BootPayload } from "./boot-payload";
import { RootErrorBoundary, RouteErrorBoundary } from "./app-error-boundary";
import { SpaRoutes } from "./spa-routes";
import { installVitePreloadRecovery } from "./vite-preload-recovery";
import { markRenderingEngine } from "@/lib/browser/rendering-engine";
import { applyTitlePrefix } from "@/lib/browser/document-title";

installVitePreloadRecovery();
markRenderingEngine(document.documentElement);

const AUTH_ROUTE_PATHS = new Set(["/login", "/setup", "/invite"]);

/** Registers the 401 handler once: clear the auth slice and bounce to
 * /login, guarding against redirect loops when already on an auth page. */
function useUnauthorizedRedirect() {
  const store = useAppStoreApi();
  useEffect(() => {
    setOnUnauthorized(() => {
      store.getState().clearAuthenticated();
      if (!AUTH_ROUTE_PATHS.has(window.location.pathname)) {
        window.location.assign("/login");
      }
    });
    return () => setOnUnauthorized(null);
  }, [store]);
}

function AppBody({ payload }: { payload: BootPayload }) {
  useUnauthorizedRedirect();
  const decision = useAuthGateDecision();
  if (decision !== "app") {
    return <AuthGatedScreen decision={decision} />;
  }
  return (
    <>
      <PluginBootBridge plugins={payload.plugins} />
      <PluginModalHost />
      <AppShell>
        <RouteErrorBoundary>
          <SpaRoutes routeData={payload.routeData} />
        </RouteErrorBoundary>
      </AppShell>
    </>
  );
}

function App({ payload }: { payload: BootPayload }) {
  return (
    <StateProvider initialState={payload.initialState ?? {}}>
      <AppBody payload={payload} />
    </StateProvider>
  );
}

const root = document.getElementById("root");

if (!root) {
  throw new Error("Missing #root element");
}

void loadBootPayload().then(async (payload) => {
  // The Go shell already rewrote <title>; this is the /api/v1/app-state boot
  // path, which never renders through it.
  applyTitlePrefix(payload.runtime?.titlePrefix, document);

  // Only `en` ships in the entry chunk, so a non-English boot has to fetch its
  // catalogs. Awaited HERE, in the promise the mount already waited on, rather
  // than after mounting: with `returnNull: false` a missing key renders as the
  // key itself, so rendering first would paint a frame of raw `common:save`
  // strings. Resolves immediately for `en` — the common case is unchanged.
  await preloadLocale(resolveInitialLocale(payload));

  createRoot(root).render(
    <StrictMode>
      <RootErrorBoundary>
        <App payload={payload} />
      </RootErrorBoundary>
    </StrictMode>,
  );
});
