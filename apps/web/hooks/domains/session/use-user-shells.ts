import { useEffect } from "react";
import { useQuery } from "@tanstack/react-query";
import { useAppStore, useAppStoreApi } from "@/components/state-provider";
import { userShellsQueryOptions } from "@/lib/query/query-options";
import type { UserShellInfo } from "@/lib/state/slices";

interface UseUserShellsReturn {
  shells: UserShellInfo[];
  isLoading: boolean;
  isLoaded: boolean;
  addShell: (shell: UserShellInfo) => void;
  removeShell: (terminalId: string) => void;
}

const EMPTY_SHELLS: UserShellInfo[] = [];

/**
 * Hook to fetch and manage user shell terminals for a task environment.
 *
 * `taskId` is required for the backend's DB-backed ordinary-terminal path
 * to fire. Without it, only the legacy passthrough shells (bottom-panel,
 * scripts) come back — first-class persistent terminals would never reach
 * the panel strip, and the parked-terminals submenu would always be
 * empty.
 *
 * Follows the data fetching pattern:
 * 1. Read from store first
 * 2. Fetch from backend if not loaded
 * 3. Track loading/loaded state
 */
export function useUserShells(
  environmentId: string | null,
  taskId?: string | null,
): UseUserShellsReturn {
  const store = useAppStoreApi();
  const connectionStatus = useAppStore((state) => state.connection.status);
  const shellsQuery = useQuery({
    ...userShellsQueryOptions(environmentId ?? "", taskId),
    enabled: Boolean(environmentId && connectionStatus === "connected"),
  });

  const storeShells = useAppStore((state) => {
    if (!environmentId) return EMPTY_SHELLS;
    return state.userShells.byEnvironmentId[environmentId] ?? EMPTY_SHELLS;
  });
  const storeLoading = useAppStore((state) => {
    if (!environmentId) return false;
    return state.userShells.loading[environmentId] ?? false;
  });
  const storeLoaded = useAppStore((state) => {
    if (!environmentId) return false;
    return state.userShells.loaded[environmentId] ?? false;
  });
  const shells = shellsQuery.data ?? storeShells;
  const isLoading = shellsQuery.isFetching || storeLoading;
  const isLoaded = Boolean(shellsQuery.data) || storeLoaded;

  useEffect(() => {
    if (!environmentId || !shellsQuery.data) return;
    store.getState().setUserShells(environmentId, shellsQuery.data);
  }, [environmentId, shellsQuery.data, store]);

  const addShell = (shell: UserShellInfo) => {
    if (environmentId) {
      store.getState().addUserShell(environmentId, shell);
    }
  };

  const removeShell = (terminalId: string) => {
    if (environmentId) {
      store.getState().removeUserShell(environmentId, terminalId);
    }
  };

  return {
    shells,
    isLoading,
    isLoaded,
    addShell,
    removeShell,
  };
}
