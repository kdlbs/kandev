import { StrictMode } from "react";
import { createRoot } from "react-dom/client";
import "@/app/globals.css";
import { StateProvider } from "@/components/state-provider";
import { AppShell } from "./app-shell";
import { readBootPayload } from "./boot-payload";
import { SpaRoutes } from "./spa-routes";

function App() {
  const payload = readBootPayload();

  return (
    <StateProvider initialState={payload.initialState ?? {}}>
      <AppShell>
        <SpaRoutes />
      </AppShell>
    </StateProvider>
  );
}

const root = document.getElementById("root");

if (!root) {
  throw new Error("Missing #root element");
}

createRoot(root).render(
  <StrictMode>
    <App />
  </StrictMode>,
);
