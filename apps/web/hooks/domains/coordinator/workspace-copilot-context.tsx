import { createContext, useContext, useMemo, type ReactNode } from "react";
import { useStore } from "zustand";
import {
  createWorkspaceCopilotStore,
  type WorkspaceCopilotStore,
} from "@/hooks/domains/coordinator/workspace-copilot-store";

const WorkspaceCopilotStoreContext = createContext<WorkspaceCopilotStore | null>(null);

/** Never written: stands in for the host store where no host is mounted. */
const ABSENT_HOST_STORE = createWorkspaceCopilotStore();

export function WorkspaceCopilotStoreProvider({
  store,
  children,
}: {
  store: WorkspaceCopilotStore;
  children: ReactNode;
}) {
  return (
    <WorkspaceCopilotStoreContext.Provider value={store}>
      {children}
    </WorkspaceCopilotStoreContext.Provider>
  );
}

export function useWorkspaceCopilotStore(): WorkspaceCopilotStore | null {
  return useContext(WorkspaceCopilotStoreContext);
}

/** How the board preview shares the right-hand slot with the workspace copilot.
 *  Without a host (flag off) the preview never yields and every claim is a no-op. */
export function useRightPanelBridge(): {
  copilotHoldsPanel: boolean;
  claimPreview: () => void;
  releasePreview: () => void;
} {
  const store = useWorkspaceCopilotStore();
  const copilotHoldsPanel = useStore(store ?? ABSENT_HOST_STORE, (s) => s.rightPanel === "copilot");
  const actions = useMemo(
    () => ({
      claimPreview: () => store?.getState().claimPreview(),
      releasePreview: () => store?.getState().releasePreview(),
    }),
    [store],
  );
  return { copilotHoldsPanel: store !== null && copilotHoldsPanel, ...actions };
}
