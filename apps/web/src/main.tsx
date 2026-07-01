import { StrictMode } from "react";
import { createRoot } from "react-dom/client";
import type { QueryClient } from "@tanstack/react-query";
import "@/app/globals.css";
import { StateProvider } from "@/components/state-provider";
import { getBrowserQueryClient } from "@/lib/query/client";
import { QueryProvider } from "@/lib/query/provider";
import { seedQueryClientFromBootPayload } from "@/lib/query/seed";
import { installWsAccountGlobalsForE2E } from "@/lib/ws/ws-account";
import { AppShell } from "./app-shell";
import { loadBootPayload } from "./boot-payload";
import type { BootPayload } from "./boot-payload";
import { SpaRoutes } from "./spa-routes";

function App({ payload, queryClient }: { payload: BootPayload; queryClient: QueryClient }) {
  return (
    <QueryProvider client={queryClient}>
      <StateProvider initialState={payload.initialState ?? {}}>
        <AppShell>
          <SpaRoutes routeData={payload.routeData} />
        </AppShell>
      </StateProvider>
    </QueryProvider>
  );
}

const root = document.getElementById("root");

if (!root) {
  throw new Error("Missing #root element");
}

installWsAccountGlobalsForE2E();

void loadBootPayload().then((payload) => {
  const queryClient = getBrowserQueryClient();
  seedQueryClientFromBootPayload(queryClient, payload);

  createRoot(root).render(
    <StrictMode>
      <App payload={payload} queryClient={queryClient} />
    </StrictMode>,
  );
});
