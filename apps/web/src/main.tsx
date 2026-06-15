import { StrictMode } from "react";
import { createRoot } from "react-dom/client";
import "@/app/globals.css";
import { PageClient } from "@/app/page-client";
import { StateProvider } from "@/components/state-provider";
import { readBootPayload } from "./boot-payload";
import { getInitialPageProps } from "./spa-routing";

function App() {
  const payload = readBootPayload();
  const initialPageProps = getInitialPageProps(payload);

  return (
    <StateProvider initialState={payload.initialState ?? {}}>
      <PageClient {...initialPageProps} />
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
