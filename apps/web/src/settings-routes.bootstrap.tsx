import { useEffect, useRef } from "react";
import { useAppStoreApi } from "@/components/state-provider";
import { loadSettingsInitialState } from "./settings-routes.initial-state";

export function SettingsRouteBootstrap({ pathname }: { pathname: string }) {
  const store = useAppStoreApi();
  const bootstrappedRef = useRef(false);

  useEffect(() => {
    if (bootstrappedRef.current) return;
    bootstrappedRef.current = true;
    let cancelled = false;

    async function bootstrap() {
      const initialState = await loadSettingsInitialState(
        () => store.getState().agentProfiles.version,
        () => store.getState().workspaces.activeId,
      );
      if (cancelled || Object.keys(initialState).length === 0) return;
      const desiredWorkspaceId = initialState.workspaces?.activeId ?? null;
      const workspaceBeforeHydration = store.getState().workspaces.activeId;
      // Routing the actual switch through `setActiveWorkspace` (rather than
      // letting `hydrate` overwrite `activeId` directly) keeps
      // `activeIdRevision` accurate for consumers that key staleness off it,
      // such as the Failed-inbox cache.
      store.getState().hydrate(
        initialState.workspaces
          ? {
              ...initialState,
              workspaces: { ...initialState.workspaces, activeId: workspaceBeforeHydration },
            }
          : initialState,
      );
      if (initialState.workspaces && desiredWorkspaceId !== workspaceBeforeHydration) {
        store.getState().setActiveWorkspace(desiredWorkspaceId);
      }
    }

    void bootstrap();
    return () => {
      cancelled = true;
      bootstrappedRef.current = false;
    };
  }, [pathname, store]);

  return null;
}
