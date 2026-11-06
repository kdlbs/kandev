"use client";

import { useEffect, useMemo } from "react";
import type { StoreApi } from "zustand";
import { useAppStore, useAppStoreApi } from "@/components/state-provider";
import type { DockviewApi } from "dockview-react";
import { useDockviewStore } from "@/lib/state/dockview-store";
import { TERMINAL_DEFAULT_ID } from "@/lib/state/layout-manager";
import { removeInvalidOrdinaryTerminalPanels } from "@/components/task/dockview-layout-setup";
import { createUserShell } from "@/lib/api/domains/user-shell-api";
import type { AppState } from "@/lib/state/store";
import type { UserShellInfo } from "@/lib/state/slices/session-runtime/types";
import {
  captureUserShellPublicationOwner,
  isActiveUserShellPublicationOwner,
  makeUserShellScope,
  userShellScopeKey,
  type UserShellScope,
} from "@/lib/state/user-shell-scope";

/**
 * Creation ownership for the default panel, per app store and scope. Several
 * consumers render the terminal pane at once (dockview header, mobile pane,
 * picker), so a module-level promise is the only way they join the same
 * create instead of minting several terminals.
 */
const pendingCreations = new WeakMap<StoreApi<AppState>, Map<string, Promise<string>>>();

function creationGuard(store: StoreApi<AppState>): Map<string, Promise<string>> {
  let guard = pendingCreations.get(store);
  if (!guard) {
    guard = new Map();
    pendingCreations.set(store, guard);
  }
  return guard;
}

function eligibleOrdinaryShells(shells: UserShellInfo[] | null): UserShellInfo[] {
  return (shells ?? [])
    .filter((shell) => shell.kind === "ordinary" && shell.state !== "parked")
    .sort((a, b) => (a.seq ?? 0) - (b.seq ?? 0));
}

function panelParamsOf(panel: unknown): Record<string, unknown> {
  return (panel as { params?: Record<string, unknown> } | undefined)?.params ?? {};
}

function panelTerminalId(panel: unknown): string | undefined {
  return panelParamsOf(panel).terminalId as string | undefined;
}

/** The restored default panel must carry the active task/environment stamps. */
function panelStampsMatch(panel: unknown, scope: UserShellScope): boolean {
  const params = panelParamsOf(panel);
  const stampedTask = params.taskID as string | undefined;
  const stampedEnvironment = params.environmentId as string | undefined;
  if (stampedTask && stampedTask !== scope.taskId) return false;
  if (stampedEnvironment && stampedEnvironment !== scope.environmentId) return false;
  return true;
}

/**
 * Applies a terminal selection and the active task/environment stamps to the
 * default panel. The panel must still be the same instance, still point at the
 * terminal the caller observed, so a late result cannot rewrite a reused or
 * navigated-away panel.
 */
function selectDefaultPanel(
  api: DockviewApi,
  panel: unknown,
  terminalId: string,
  expectedTerminalId: string | undefined,
  scope: UserShellScope,
): void {
  const live = api.getPanel(TERMINAL_DEFAULT_ID);
  if (!live || live !== panel || panelTerminalId(live) !== expectedTerminalId) return;
  live.api.updateParameters({
    terminalId,
    environmentId: scope.environmentId,
    taskID: scope.taskId,
  });
}

/** Creation authority must still hold at the mutation boundary. */
function canCreateForScope(store: StoreApi<AppState>, scopeKey: string): boolean {
  const shellState = store.getState().userShells;
  return shellState.listed[scopeKey] === true && shellState.loading[scopeKey] !== true;
}

function shellFromCreateResult(result: Awaited<ReturnType<typeof createUserShell>>): UserShellInfo {
  return {
    terminalId: result.terminalId,
    kind: result.kind,
    seq: result.seq,
    displayName: result.displayName,
    customName: null,
    state: result.state ?? "open",
    ptyStatus: result.ptyStatus ?? "stopped",
    running: result.ptyStatus === "running",
    label: result.label,
    closable: result.closable ?? true,
    initialCommand: result.initialCommand,
  };
}

function createOnce(
  store: StoreApi<AppState>,
  scope: UserShellScope,
  scopeKey: string,
  authorized: () => boolean,
): Promise<string> | null {
  if (!authorized()) return null;
  const guard = creationGuard(store);
  const existing = guard.get(scopeKey);
  if (existing) return existing;
  const creation = createUserShell(scope.environmentId, { taskId: scope.taskId ?? undefined })
    .then((result) => {
      // The admitted create publishes to the cache it was admitted for, even
      // if the user has navigated away by now.
      store.getState().addUserShell(scope, shellFromCreateResult(result));
      return result.terminalId;
    })
    .finally(() => {
      guard.delete(scopeKey);
    });
  guard.set(scopeKey, creation);
  return creation;
}

/**
 * Reconciles the `terminal-default` panel with the current task's ordinary
 * terminals.
 *
 * - A valid open ordinary selection stays selected.
 * - A restored panel pointing at a parked, foreign, or unknown ID is repaired
 *   from the current scope, never from another task's cache.
 * - After a successful empty eligible list, exactly one consumer creates a
 *   replacement. Pending and failed reads create nothing.
 * - A late create completion updates only its original cache; it cannot
 *   rewrite a reused panel or create a panel the layout omitted.
 */
export function useEnsureDefaultTerminalOrdinary(): void {
  const store = useAppStoreApi();
  const environmentId = useAppStore((s) => {
    const sid = s.tasks?.activeSessionId;
    return sid ? (s.environmentIdBySessionId[sid] ?? null) : null;
  });
  const taskID = useAppStore((s) => s.tasks?.activeTaskId ?? null);
  const scope = useMemo(() => makeUserShellScope(environmentId, taskID), [environmentId, taskID]);
  const scopeKey = scope ? userShellScopeKey(scope) : "";
  const hasSuccessfulList = useAppStore((s) => {
    if (!scopeKey) return false;
    return s.userShells.listed[scopeKey] ?? false;
  });
  const isLoading = useAppStore((s) => {
    if (!scopeKey) return false;
    return s.userShells.loading[scopeKey] ?? true;
  });
  const shells = useAppStore((s) => {
    if (!scopeKey) return null;
    return s.userShells.byScope[scopeKey] ?? null;
  });
  const dockviewApi = useDockviewStore((s) => s.api);

  useEffect(() => {
    // Ordinary terminals require a real task. A taskless scope can never
    // prove ownership or authorize creation.
    if (!scope?.taskId || !scopeKey || !dockviewApi) return;
    // Re-read authority at the boundary: a read may have started after this
    // effect was scheduled, which withdraws the older successful evidence.
    if (!hasSuccessfulList || isLoading || !canCreateForScope(store, scopeKey)) return;
    // Extra ordinary panels restored from a layout may point at another
    // task's terminal. They must stop connecting before the panel below is
    // reconciled.
    const owner = captureUserShellPublicationOwner(store.getState(), scope);
    removeInvalidOrdinaryTerminalPanels(dockviewApi, store, scope);
    const panel = dockviewApi.getPanel(TERMINAL_DEFAULT_ID);
    // Layouts that omit the default panel must stay free of an automatic
    // desktop terminal.
    if (!panel) return;

    const currentTerminalId = panelTerminalId(panel);
    const eligible = eligibleOrdinaryShells(shells);
    // A restored panel may carry no terminal id at all (its params were
    // stripped during layout normalization). That panel needs a selection just
    // like a foreign or parked id does.
    if (currentTerminalId && eligible.some((shell) => shell.terminalId === currentTerminalId)) {
      // The terminal is owned but the restored panel may still carry another
      // task's or environment's stamp. Repair the stamps in place.
      if (!panelStampsMatch(panel, scope))
        selectDefaultPanel(dockviewApi, panel, currentTerminalId, currentTerminalId, scope);
      return;
    }

    if (eligible.length > 0) {
      selectDefaultPanel(dockviewApi, panel, eligible[0].terminalId, currentTerminalId, scope);
      return;
    }

    const creation = createOnce(
      store,
      scope,
      scopeKey,
      () =>
        isActiveUserShellPublicationOwner(store.getState(), owner) &&
        canCreateForScope(store, scopeKey),
    );
    creation
      ?.then((terminalId) => {
        // Recheck after the asynchronous create: the user may have navigated
        // or the panel may have been reused for another terminal.
        if (!isActiveUserShellPublicationOwner(store.getState(), owner)) return;
        selectDefaultPanel(dockviewApi, panel, terminalId, currentTerminalId, scope);
      })
      .catch((error: unknown) => {
        console.error("default-terminal migration:", error);
      });
  }, [store, scope, scopeKey, hasSuccessfulList, isLoading, shells, dockviewApi]);
}
