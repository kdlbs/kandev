import { StrictMode } from "react";
import { createRoot } from "react-dom/client";
import "@/app/globals.css";
import { StateProvider } from "@/components/state-provider";
import { PluginBootBridge } from "@/lib/plugins/plugin-boot-bridge";
import { AppShell } from "./app-shell";
import { loadBootPayload } from "./boot-payload";
import type { BootPayload } from "./boot-payload";
import { SpaRoutes } from "./spa-routes";

function App({ payload }: { payload: BootPayload }) {
  return (
    <StateProvider initialState={payload.initialState ?? {}}>
      <PluginBootBridge plugins={payload.plugins} />
      <AppShell>
        <SpaRoutes routeData={payload.routeData} />
      </AppShell>
    </StateProvider>
  );
}

const root = document.getElementById("root");

if (!root) {
  throw new Error("Missing #root element");
}

void loadBootPayload().then((payload) => {
  createRoot(root).render(
    <StrictMode>
      <App payload={payload} />
    </StrictMode>,
  );
});
