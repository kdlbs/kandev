import { useEffect, useRef } from "react";
import { getWebSocketClient } from "@/lib/ws/connection";
import { useAppStore, useAppStoreApi } from "@/components/state-provider";
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
 * User shells are env-scoped — sessions in the same task share one shell list.
 * Pass the environment id directly; do not pass a session id.
 *
 * Follows the data fetching pattern:
 * 1. Read from store first
 * 2. Fetch from backend if not loaded
 * 3. Track loading/loaded state
 */
export function useUserShells(environmentId: string | null): UseUserShellsReturn {
  const store = useAppStoreApi();

  const shells = useAppStore((state) => {
    if (!environmentId) return EMPTY_SHELLS;
    return state.userShells.byEnvironmentId[environmentId] ?? EMPTY_SHELLS;
  });
  const isLoading = useAppStore((state) => {
    if (!environmentId) return false;
    return state.userShells.loading[environmentId] ?? false;
  });
  const isLoaded = useAppStore((state) => {
    if (!environmentId) return false;
    return state.userShells.loaded[environmentId] ?? false;
  });
  const connectionStatus = useAppStore((state) => state.connection.status);

  // Guard ref to prevent duplicate fetches per env
  const lastFetchedEnvIdRef = useRef<string | null>(null);

  // Reset ref when environmentId clears
  useEffect(() => {
    if (!environmentId) {
      lastFetchedEnvIdRef.current = null;
    }
  }, [environmentId]);

  // Fetch user shells from backend
  useEffect(() => {
    if (!environmentId) return;
    if (connectionStatus !== "connected") return;
    if (isLoaded) {
      lastFetchedEnvIdRef.current = environmentId;
      return;
    }
    if (lastFetchedEnvIdRef.current === environmentId) return;

    const fetchShells = async () => {
      const client = getWebSocketClient();
      if (!client) return;

      store.getState().setUserShellsLoading(environmentId, true);

      try {
        const response = await client.request<{
          shells?: Array<{
            terminal_id: string;
            process_id: string;
            running: boolean;
            label: string;
            closable: boolean;
            initial_command?: string;
          }>;
        }>("user_shell.list", { task_environment_id: environmentId }, 10000);

        const shells: UserShellInfo[] = (response.shells ?? []).map((s) => ({
          terminalId: s.terminal_id,
          processId: s.process_id,
          running: s.running,
          label: s.label || "Terminal",
          closable: s.closable,
          initialCommand: s.initial_command,
        }));

        store.getState().setUserShells(environmentId, shells);
        lastFetchedEnvIdRef.current = environmentId;
      } catch (error) {
        console.error("Failed to fetch user shells:", error);
        store.getState().setUserShells(environmentId, []);
        lastFetchedEnvIdRef.current = environmentId;
      }
    };

    fetchShells();
  }, [environmentId, connectionStatus, isLoaded, store]);

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
