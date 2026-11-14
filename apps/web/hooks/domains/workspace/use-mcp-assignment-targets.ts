"use client";

import { useCallback, useEffect, useRef, useState } from "react";
import { listRepositories } from "@/lib/api/domains/workspace-api";
import { listMCPServers, listMCPProfileOwners } from "@/lib/api/domains/mcp-api";
import { listTaskSessions } from "@/lib/api/domains/session-api";
import { fetchTask } from "@/lib/api/domains/kanban-api";
import { appendMCPAssignmentHint } from "@/lib/mcp/assignment-hint";
import { mcpOwnerDestinationHref } from "@/lib/mcp/management-links";
import {
  profileAssignmentTargets,
  repositoryAssignmentTargets,
  sessionAssignmentTargets,
  type MCPAssignmentTarget,
} from "@/lib/mcp/assignment-targets";

export type MCPAssignmentScope = MCPAssignmentTarget["scope"];
type Context = { workspaceId: string; scope: MCPAssignmentScope | null; taskId?: string };
type OpenStatus = "idle" | "checking" | "failed" | "unavailable";

function targetBelongsToContext(target: MCPAssignmentTarget, context: Context) {
  return (
    target.workspaceId === context.workspaceId &&
    target.scope === context.scope &&
    (target.scope !== "task_session" || target.taskId === context.taskId)
  );
}

const INVALID_PROFILE_PAGE = Symbol();
async function readProfileTargets(workspaceId: string, signal: AbortSignal) {
  const targets: MCPAssignmentTarget[] = [];
  const cursors = new Set<string>();
  let cursor: string | undefined;
  do {
    signal.throwIfAborted();
    const page = await listMCPProfileOwners(workspaceId, cursor, { init: { signal } });
    if (page.workspace_id !== workspaceId) throw INVALID_PROFILE_PAGE;
    targets.push(...profileAssignmentTargets(workspaceId, page.items));
    cursor = page.next_cursor;
    if (cursor && cursors.has(cursor)) throw INVALID_PROFILE_PAGE;
    if (cursor) cursors.add(cursor);
  } while (cursor);
  return [...new Map(targets.map((target) => [target.ownerId, target])).values()];
}

async function readTargets({ workspaceId, scope, taskId }: Context, signal: AbortSignal) {
  const options = { cache: "no-store" as const, init: { signal } };
  if (scope === "repository") {
    const response = await listRepositories(workspaceId, undefined, options);
    return repositoryAssignmentTargets(workspaceId, response.repositories);
  }
  if (scope === "profile") {
    return readProfileTargets(workspaceId, signal);
  }
  if (scope !== "task_session" || !taskId) return [];
  const task = await fetchTask(taskId, options);
  if (task.workspace_id !== workspaceId || task.archived_at || task.workspace_orphaned) return [];
  const [response, profiles] = await Promise.all([
    listTaskSessions(taskId, options),
    readProfileTargets(workspaceId, signal),
  ]);
  return sessionAssignmentTargets(workspaceId, task, response.sessions, profiles);
}

function useAssignmentOpen(context: Context, key: string) {
  const [state, setState] = useState<{ key: string; status: OpenStatus }>({ key, status: "idle" });
  const gate = useRef<{ busy: boolean; controller: AbortController | null }>({
    busy: false,
    controller: null,
  });
  const currentKey = useRef(key);
  currentKey.current = key;
  useEffect(() => {
    gate.current = { busy: false, controller: null };
    return () => {
      gate.current.controller?.abort();
    };
  }, [key]);
  const { workspaceId, scope, taskId } = context;
  const openHref = useCallback(
    async (target: MCPAssignmentTarget, definitionId: string) => {
      if (
        gate.current.busy ||
        currentKey.current !== key ||
        !scope ||
        !targetBelongsToContext(target, { workspaceId, scope, taskId })
      )
        return null;
      const controller = new AbortController();
      gate.current = { busy: true, controller };
      const current = () =>
        !controller.signal.aborted &&
        currentKey.current === key &&
        gate.current.controller === controller;
      setState({ key, status: "checking" });
      try {
        const [definitions, targets] = await Promise.all([
          listMCPServers(workspaceId, { cache: "no-store", init: { signal: controller.signal } }),
          readTargets({ workspaceId, scope, taskId }, controller.signal),
        ]);
        if (!current()) return null;
        const owner = targets.find(
          (item) => item.scope === target.scope && item.ownerId === target.ownerId,
        );
        const definition = definitions.find(
          (item) => item.id === definitionId && item.workspace_id === workspaceId && item.enabled,
        );
        const href = owner && definition ? mcpOwnerDestinationHref(owner) : null;
        setState({ key, status: href ? "idle" : "unavailable" });
        return href ? appendMCPAssignmentHint(href, definitionId) : null;
      } catch {
        if (current()) setState({ key, status: "failed" });
        return null;
      } finally {
        if (current()) gate.current.busy = false;
      }
    },
    [key, workspaceId, scope, taskId],
  );
  return { openStatus: state.key === key ? state.status : ("idle" as OpenStatus), openHref };
}

type TargetState = {
  key: string;
  targets: MCPAssignmentTarget[];
  loading: boolean;
  failed: boolean;
};
export function useMCPAssignmentTargets(
  workspaceId: string,
  scope: MCPAssignmentScope | null,
  taskId?: string,
) {
  const [attempt, setAttempt] = useState(0);
  const [state, setState] = useState<TargetState | null>(null);
  const key = JSON.stringify([workspaceId, scope, taskId, attempt]);
  const currentKey = useRef(key);
  currentKey.current = key;
  const active = Boolean(workspaceId && scope && (scope !== "task_session" || taskId));
  useEffect(() => {
    if (!active) return;
    const controller = new AbortController();
    const current = () => !controller.signal.aborted && currentKey.current === key;
    setState({ key, targets: [], loading: true, failed: false });
    void readTargets({ workspaceId, scope, taskId }, controller.signal).then(
      (targets) => {
        if (current()) setState({ key, targets, loading: false, failed: false });
      },
      () => {
        if (current()) setState({ key, targets: [], loading: false, failed: true });
      },
    );
    return () => controller.abort();
  }, [active, key, workspaceId, scope, taskId]);
  const reload = useCallback(() => setAttempt((value) => value + 1), []);
  const open = useAssignmentOpen({ workspaceId, scope, taskId }, key);
  const visible = active && state?.key === key ? state : null;
  return {
    targets: visible?.targets ?? [],
    loading: active && (visible?.loading ?? true),
    failed: visible?.failed ?? false,
    reload,
    ...open,
  };
}
