import { useEffect } from "react";
import { useAppStore } from "@/components/state-provider";

type SessionEditorCapabilityStatus = {
  capabilities?: {
    embedded_vscode?: boolean;
  };
};

export function getEmbeddedVscodeSupported(
  status: SessionEditorCapabilityStatus | null | undefined,
): boolean {
  return status?.capabilities?.embedded_vscode === true;
}

/**
 * Resolves the session's embedded-editor capability and publishes it to the
 * store.
 *
 * The capability is fetched per session by `useSessionResumption` and held in
 * that hook's local state, which only the task page can read. Panels rendered
 * inside the layout — the Files tree context menu — need the same answer to
 * decide whether to offer the embedded editor, and they read the store rather
 * than taking props through the dock layout.
 */
export function useEmbeddedVscodeSupport(
  sessionId: string | null | undefined,
  status: SessionEditorCapabilityStatus | null | undefined,
): boolean {
  const supported = getEmbeddedVscodeSupported(status);
  const setEmbeddedVscodeSupport = useAppStore((state) => state.setEmbeddedVscodeSupport);
  useEffect(() => {
    if (!sessionId) return;
    setEmbeddedVscodeSupport(sessionId, supported);
  }, [sessionId, supported, setEmbeddedVscodeSupport]);
  return supported;
}
