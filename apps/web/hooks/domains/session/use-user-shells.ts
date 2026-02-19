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
 * Hook to fetch and manage user shell terminals for a session.
 *
 * Follows the data fetching pattern:
 * 1. Read from store first
 * 2. Fetch from backend if not loaded
 * 3. Track loading/loaded state
 */
export function useUserShells(sessionId: string | null): UseUserShellsReturn {
  const store = useAppStoreApi();

  // Read from store
  const shells = useAppStore((state) =>
    sessionId ? (state.userShells.bySessionId[sessionId] ?? EMPTY_SHELLS) : EMPTY_SHELLS,
  );
  const isLoading = useAppStore((state) =>
    sessionId ? (state.userShells.loading[sessionId] ?? false) : false,
  );
  const isLoaded = useAppStore((state) =>
    sessionId ? (state.userShells.loaded[sessionId] ?? false) : false,
  );
  const connectionStatus = useAppStore((state) => state.connection.status);

  // Guard refs to prevent duplicate fetches
  const lastFetchedSessionIdRef = useRef<string | null>(null);
  const prevSessionIdRef = useRef<string | null>(null);

  // Reset refs when session clears
  useEffect(() => {
    if (!sessionId) {
      lastFetchedSessionIdRef.current = null;
    }
  }, [sessionId]);

  // Fetch user shells from backend
  useEffect(() => {
    if (!sessionId) return;
    if (connectionStatus !== "connected") return;

    // Detect session change to force refetch
    const sessionChanged =
      prevSessionIdRef.current !== null && prevSessionIdRef.current !== sessionId;
    prevSessionIdRef.current = sessionId;

    // If session changed, reset fetch guard
    if (sessionChanged) {
      lastFetchedSessionIdRef.current = null;
    }

    // Check if already loaded (unless session just changed)
    if (isLoaded && !sessionChanged) {
      lastFetchedSessionIdRef.current = sessionId;
      return;
    }

    // Don't fetch again if already fetched for this session
    if (lastFetchedSessionIdRef.current === sessionId) {
      return;
    }

    const fetchShells = async () => {
      const client = getWebSocketClient();
      if (!client) return;

      store.getState().setUserShellsLoading(sessionId, true);

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
        }>("user_shell.list", { session_id: sessionId }, 10000);

        // Transform backend format (snake_case) to frontend format (camelCase)
        const shells: UserShellInfo[] = (response.shells ?? []).map((s) => ({
          terminalId: s.terminal_id,
          processId: s.process_id,
          running: s.running,
          label: s.label || "Terminal",
          closable: s.closable,
          initialCommand: s.initial_command,
        }));

        store.getState().setUserShells(sessionId, shells);
        lastFetchedSessionIdRef.current = sessionId;
      } catch (error) {
        console.error("Failed to fetch user shells:", error);
        // Set empty shells on error so we don't keep retrying
        store.getState().setUserShells(sessionId, []);
        lastFetchedSessionIdRef.current = sessionId;
      }
    };

    fetchShells();
  }, [sessionId, connectionStatus, isLoaded, store]);

  // Actions
  const addShell = (shell: UserShellInfo) => {
    if (sessionId) {
      store.getState().addUserShell(sessionId, shell);
    }
  };

  const removeShell = (terminalId: string) => {
    if (sessionId) {
      store.getState().removeUserShell(sessionId, terminalId);
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
