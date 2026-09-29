import { useEffect } from "react";
import { getWebSocketClient } from "@/lib/ws/connection";
import { useAppStore, useAppStoreApi } from "@/components/state-provider";
import type {
  UserShellInfo,
  UserShellKind,
  UserShellState,
  UserShellPTYStatus,
} from "@/lib/state/slices";
import { t } from "@/lib/i18n";
import { useSessionRead } from "./use-session-read";

interface UseUserShellsReturn {
  shells: UserShellInfo[];
  isLoading: boolean;
  isLoaded: boolean;
  addShell: (shell: UserShellInfo) => void;
  removeShell: (terminalId: string) => void;
}

const EMPTY_SHELLS: UserShellInfo[] = [];

/**
 * Wire shape of an item in the `user_shell.list` response. Backend returns
 * a discriminated union — `kind: "ordinary"` carries DB metadata, `fixed`
 * and `script` only carry the id + pty_status + label.
 */
type ListResponseItem = {
  id?: string;
  terminal_id?: string; // legacy / passthrough
  kind?: UserShellKind;
  seq?: number;
  display_name?: string;
  custom_name?: string | null;
  state?: UserShellState;
  pty_status?: UserShellPTYStatus;
  label?: string;
  closable?: boolean;
  initial_command?: string;
  process_id?: string;
  running?: boolean;
};

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
  const { read, scope } = useSessionRead<UserShellInfo[]>(
    environmentId
      ? {
          resource: "shells",
          key: JSON.stringify([environmentId, taskId ?? null]),
          environmentId,
          onLoading: (loading) => store.getState().setUserShellsLoading(environmentId, loading),
          fetch: async (isCurrent) => {
            const payload: Record<string, unknown> = {
              task_environment_id: environmentId,
              include_parked: true,
            };
            if (taskId) payload.task_id = taskId;
            let mapped: UserShellInfo[];
            try {
              const response = await getWebSocketClient()!.request<{ shells?: ListResponseItem[] }>(
                "user_shell.list",
                payload,
                10000,
              );
              mapped = (response.shells ?? []).map(mapListItemToShell);
            } catch {
              // Failed reads settle terminal initialization without discarding known shells.
              mapped = store.getState().userShells.byEnvironmentId[environmentId] ?? EMPTY_SHELLS;
            }
            if (!isCurrent()) return {};
            store.getState().setUserShells(environmentId, mapped);
            return { data: mapped };
          },
        }
      : null,
  );
  useEffect(() => read?.ensure(undefined, !isLoaded), [read, scope, isLoaded]);

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

/**
 * Maps the wire shape onto the in-store `UserShellInfo`. The new handler
 * returns `id` (typed `kind: "ordinary" | "fixed" | "script"`), the
 * legacy passthrough returns `terminal_id`. Both populate the legacy
 * `processId/running/label/closable` fields when present so older
 * consumers (e.g. the `useTerminals` build path for non-ordinary tabs)
 * keep rendering correctly.
 */
function mapListItemToShell(s: ListResponseItem): UserShellInfo {
  const id = (s.id ?? s.terminal_id ?? "") as string;
  return {
    terminalId: id,
    kind: s.kind,
    seq: s.seq,
    customName: s.custom_name ?? null,
    displayName: s.display_name,
    state: s.state,
    ptyStatus: s.pty_status,
    processId: s.process_id,
    running: s.running ?? s.pty_status === "running",
    label: s.label || s.display_name || t("common:terminal"),
    closable: s.closable ?? true,
    initialCommand: s.initial_command,
  };
}
