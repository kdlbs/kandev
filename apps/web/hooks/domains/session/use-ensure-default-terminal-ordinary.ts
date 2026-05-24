"use client";

import { useEffect, useRef } from "react";
import { useAppStore } from "@/components/state-provider";
import { useDockviewStore } from "@/lib/state/dockview-store";
import { TERMINAL_DEFAULT_ID } from "@/lib/state/layout-manager";
import { createUserShell } from "@/lib/api/domains/user-shell-api";

/**
 * The legacy `terminal-default` dockview panel ships with a hardcoded
 * `terminalId: "shell-default"` that bypasses the DB-backed ordinary
 * terminal flow — no seq, no rename, no parking. That breaks the
 * promised "Terminal N" badge UX: with only the default present, the
 * user sees a plain "Terminal" tab; once they add a second one, the
 * badge logic that gates on "more than one ordinary terminal" still
 * counts only the user-created shell.
 *
 * This hook migrates the default panel into an ordinary terminal on
 * session-page mount: if `terminal-default` exists and still points at
 * `shell-default`, it calls `user_shell.create` to mint a real
 * task-scoped row and rewrites the panel's params to use the new id.
 * Subsequent loads see an ordinary terminal already in the store and
 * short-circuit.
 *
 * The migration is keyed on `env:task` so a task switch repeats it for
 * the new task. Concurrent mounts (mobile pane + dockview header) are
 * serialised by a module-level Set guard.
 */
const migratingScopes = new Set<string>();

export function useEnsureDefaultTerminalOrdinary(): void {
  const environmentId = useAppStore((s) => {
    const sid = s.tasks?.activeSessionId;
    return sid ? (s.environmentIdBySessionId[sid] ?? null) : null;
  });
  const taskID = useAppStore((s) => s.tasks?.activeTaskId ?? null);
  const userShellsLoaded = useAppStore((s) => {
    if (!environmentId) return false;
    return s.userShells.loaded[environmentId] ?? false;
  });
  const shells = useAppStore((s) => {
    if (!environmentId) return null;
    return s.userShells.byEnvironmentId[environmentId] ?? null;
  });
  const addUserShell = useAppStore((s) => s.addUserShell);
  const dockviewApi = useDockviewStore((s) => s.api);
  const lastMigratedScopeRef = useRef<string | null>(null);

  useEffect(() => {
    if (!environmentId || !taskID || !userShellsLoaded || !dockviewApi) return;
    const scope = `${environmentId}:${taskID}`;
    if (lastMigratedScopeRef.current === scope) return;

    const panel = dockviewApi.getPanel(TERMINAL_DEFAULT_ID);
    if (!panel) {
      lastMigratedScopeRef.current = scope;
      return;
    }

    // Look at panel.params via the registered params recorded at create
    // time. Dockview's IDockviewPanel exposes `.params` (Record<string,
    // unknown>) directly. The default registration stamps
    // `terminalId: "shell-default"`; once migrated we replace it.
    const panelParams = (panel as unknown as { params?: Record<string, unknown> }).params ?? {};
    const currentTerminalId = panelParams.terminalId as string | undefined;
    if (currentTerminalId !== "shell-default") {
      lastMigratedScopeRef.current = scope;
      return;
    }

    // If the task already owns ordinary terminals (from a previous
    // session-page visit), reuse the lowest-seq one for the default
    // panel instead of minting a new row.
    const existingOrdinary = (shells ?? [])
      .filter((it) => it.kind === "ordinary")
      .sort((a, b) => (a.seq ?? 0) - (b.seq ?? 0))[0];

    const finishMigration = (terminalId: string) => {
      panel.api.updateParameters({
        terminalId,
        environmentId,
        taskID,
      });
      // The panel title is intentionally left as the registry default
      // ("Terminal"). TerminalTab's own useEffect drives `api.setTitle`
      // to the live display name (custom rename → customName; otherwise
      // plain "Terminal" with the seq rendered separately in the
      // badge). Setting a "Terminal {seq}" title here would race the
      // tab component and stick.
      lastMigratedScopeRef.current = scope;
    };

    if (existingOrdinary) {
      finishMigration(existingOrdinary.terminalId);
      return;
    }

    if (migratingScopes.has(scope)) return;
    migratingScopes.add(scope);

    (async () => {
      try {
        const result = await createUserShell(environmentId, { taskId: taskID });
        addUserShell(environmentId, {
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
        });
        finishMigration(result.terminalId);
      } catch (error) {
        console.error("default-terminal migration:", error);
        // Don't lock the scope so a retry can run on the next tick.
      } finally {
        migratingScopes.delete(scope);
      }
    })();
  }, [environmentId, taskID, userShellsLoaded, shells, dockviewApi, addUserShell]);
}
