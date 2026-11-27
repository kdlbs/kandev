import { useEffect, useMemo } from "react";
import { getWebSocketClient } from "@/lib/ws/connection";
import { useAppStore, useAppStoreApi } from "@/components/state-provider";
import type {
  UserShellInfo,
  UserShellKind,
  UserShellState,
  UserShellPTYStatus,
} from "@/lib/state/slices";
import {
  makeUserShellScope,
  userShellScopeKey,
  type UserShellScope,
} from "@/lib/state/user-shell-scope";
import { t } from "@/lib/i18n";
import { useSessionRead } from "./use-session-read";

interface UseUserShellsReturn {
  shells: UserShellInfo[];
  isLoading: boolean;
  /** True once a read settled; a failed read also settles. */
  isLoaded: boolean;
  /** True only after a successful list for this scope. */
  hasSuccessfulList: boolean;
  scope: UserShellScope | null;
  addShell: (shell: UserShellInfo) => void;
  removeShell: (terminalId: string) => void;
}

const EMPTY_SHELLS: UserShellInfo[] = [];
const SHELL_LIST_RETRY_MS = 5_000;

/**
 * Per-store request generation for the environment-shared rows. Each list
 * request allocates its generation before awaiting, so a delayed older
 * response cannot replace newer fixed/script terminals from a sibling task.
 */
const sharedReadGenerations = new WeakMap<object, Map<string, number>>();

function nextSharedReadGeneration(store: object, environmentId: string): number {
  let generations = sharedReadGenerations.get(store);
  if (!generations) {
    generations = new Map();
    sharedReadGenerations.set(store, generations);
  }
  const next = (generations.get(environmentId) ?? 0) + 1;
  generations.set(environmentId, next);
  return next;
}

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
 * Fetches and manages user shell terminals for one task/environment scope.
 *
 * Ordinary terminals are task-owned: two tasks sharing an environment keep
 * independent result sets, while sessions of the same task and environment
 * share one. Fixed and script terminals remain environment-shared and are
 * merged into the returned list from their own namespace.
 *
 * `taskId` is required for the backend's DB-backed ordinary-terminal path to
 * fire. Without it only legacy passthrough shells (bottom-panel, scripts)
 * come back and ordinary rows are never adopted into an owned scope.
 */
export function useUserShells(
  environmentId: string | null,
  taskId?: string | null,
): UseUserShellsReturn {
  const store = useAppStoreApi();
  const scope = makeUserShellScope(environmentId, taskId);
  const scopeKey = scope ? userShellScopeKey(scope) : "";

  const ordinary = useAppStore((state) => {
    if (!scopeKey) return EMPTY_SHELLS;
    return state.userShells.byScope[scopeKey] ?? EMPTY_SHELLS;
  });
  const unmanaged = useAppStore((state) => {
    if (!environmentId) return EMPTY_SHELLS;
    return state.userShells.byEnvironmentId[environmentId] ?? EMPTY_SHELLS;
  });
  // Ordinary rows and environment-shared rows are separate namespaces; merge
  // them into one stable value so consumers do not re-render on every read.
  const shells = useMemo(() => {
    if (ordinary.length === 0) return unmanaged;
    if (unmanaged.length === 0) return ordinary;
    return [...ordinary, ...unmanaged];
  }, [ordinary, unmanaged]);

  const isLoading = useAppStore((state) => {
    if (!scopeKey) return false;
    return state.userShells.loading[scopeKey] ?? false;
  });
  const isLoaded = useAppStore((state) => {
    if (!scopeKey) return false;
    return state.userShells.loaded[scopeKey] ?? false;
  });
  const isListed = useAppStore((state) => {
    if (!scopeKey) return false;
    return state.userShells.listed[scopeKey] ?? false;
  });

  const { read, snapshot } = useSessionRead<UserShellInfo[]>(
    scope
      ? {
          resource: "shells",
          key: scopeKey,
          environmentId: scope.environmentId,
          publishKey: scopeKey,
          onLoading: (loading) => store.getState().setUserShellsLoading(scope, loading),
          fetch: async (isCurrent) => {
            const sharedGeneration = nextSharedReadGeneration(store, scope.environmentId);
            const payload: Record<string, unknown> = {
              task_environment_id: scope.environmentId,
              include_parked: true,
            };
            if (scope.taskId) payload.task_id = scope.taskId;
            try {
              const response = await getWebSocketClient()!.request<{ shells?: ListResponseItem[] }>(
                "user_shell.list",
                payload,
                10000,
              );
              const mapped = (response.shells ?? []).map(mapListItemToShell);
              if (!isCurrent()) return {};
              store.getState().setUserShells(scope, mapped, sharedGeneration);
              return { data: mapped };
            } catch {
              if (!isCurrent()) return {};
              // Failed reads settle initialization without discarding known
              // shells, and they do not become successful-list evidence.
              store.getState().setUserShellsFailed(scope);
              return {};
            }
          },
        }
      : null,
  );
  // Selection and creation authority belong to the successful read for the
  // current scope generation. A store flag from an older successful read is
  // not enough: reconnect, restoration, and refetch all start a new read that
  // may change the result.
  const hasSuccessfulList =
    !!scopeKey && isListed && !isLoading && snapshot.data !== undefined && !snapshot.loading;
  useEffect(() => read?.ensure(undefined, !isLoaded), [read, scopeKey, isLoaded]);

  // A failed list keeps display continuity but no authority. Retry while the
  // consumer remains mounted so a transient WS/API failure can recover.
  useEffect(() => {
    if (!read || !isLoaded || isLoading || hasSuccessfulList) return;
    const timer = setTimeout(() => read.invalidate(), SHELL_LIST_RETRY_MS);
    return () => clearTimeout(timer);
  }, [read, isLoaded, isLoading, hasSuccessfulList]);

  const addShell = (shell: UserShellInfo) => {
    if (scope) store.getState().addUserShell(scope, shell);
  };

  const removeShell = (terminalId: string) => {
    if (!scope) return;
    store.getState().removeUserShell(scope, terminalId);
  };

  return {
    shells,
    isLoading,
    isLoaded,
    hasSuccessfulList,
    scope,
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
