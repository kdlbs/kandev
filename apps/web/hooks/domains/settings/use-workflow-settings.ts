"use client";

import { useEffect, useMemo, useRef, useState } from "react";
import { useAllCachedWorkflows, useCachedWorkflows } from "@/hooks/use-workflow-cache";
import { useWorkflows } from "@/hooks/use-workflows";
import { workflowId, workspaceId as toWorkspaceId, type Workflow } from "@/lib/types/http";

/**
 * Manages workflow list state for the settings page, synced with Query cache
 * updates. Supports local edits (dirty tracking) and temp drafts.
 *
 * `workspaceId` scopes the visible workflows to the current workspace so that
 * stale entries from previously visited workspaces (still cached in Query)
 * don't leak into another workspace's settings page.
 */
export function useWorkflowSettings(initialWorkflows: Workflow[], workspaceId?: string) {
  useWorkflows(workspaceId ?? null, Boolean(workspaceId));
  const workspaceCachedWorkflows = useCachedWorkflows(workspaceId);
  const allCachedWorkflows = useAllCachedWorkflows();
  const cachedWorkflows = workspaceId ? workspaceCachedWorkflows : allCachedWorkflows;
  // Hidden workflows (e.g. the system "Improve Kandev" template) and
  // office-style workflows are loaded into Query with `includeHidden: true`
  // so other surfaces can resolve references, but the workflow settings page
  // is kanban-only. Filter them out at the cache/prop boundaries so all
  // downstream merging logic remains settings-visible only.
  const visibleInitialWorkflows = useMemo(
    () => initialWorkflows.filter(isSettingsVisibleWorkflow),
    [initialWorkflows],
  );
  const visibleCachedWorkflows = useMemo(
    () => cachedWorkflows.filter(isSettingsVisibleWorkflow),
    [cachedWorkflows],
  );
  const [workflowItems, setWorkflowItems] = useState<Workflow[]>(visibleInitialWorkflows);
  const [savedWorkflowItems, setSavedWorkflowItems] = useState<Workflow[]>(visibleInitialWorkflows);

  // Track all IDs we've ever seen from SSR props so we only add genuinely new ones
  // (not re-add workflows the user deleted locally).
  const seenInitialIdsRef = useRef<Set<string>>(new Set(visibleInitialWorkflows.map((w) => w.id)));

  // Merge new workflows from SSR props (e.g. after router.refresh() following import).
  // useState ignores updated initialWorkflows on re-render, so we sync manually.
  useEffect(() => {
    const seen = seenInitialIdsRef.current;
    const newWorkflows = visibleInitialWorkflows.filter((w) => !seen.has(w.id));
    if (newWorkflows.length === 0) return;

    for (const w of newWorkflows) seen.add(w.id);

    setWorkflowItems((prev) => {
      const localIds = new Set(prev.map((w) => w.id));
      const toAdd = newWorkflows.filter((w) => !localIds.has(w.id));
      if (toAdd.length === 0) return prev;
      return [...prev, ...toAdd];
    });
    setSavedWorkflowItems((prev) => {
      const localIds = new Set(prev.map((w) => w.id));
      const toAdd = newWorkflows.filter((w) => !localIds.has(w.id));
      if (toAdd.length === 0) return prev;
      return [...prev, ...toAdd];
    });
  }, [visibleInitialWorkflows]);

  // Track which IDs the cache has previously reported so we only remove
  // workflows that were actually deleted, not ones the cache never knew about.
  const prevCachedIdsRef = useRef<Set<string>>(new Set());

  useEffect(() => {
    const currentCachedIds = new Set(visibleCachedWorkflows.map((w) => w.id));
    const prevCachedIds = prevCachedIdsRef.current;

    // IDs that were in the cache last render but are gone now → actually deleted.
    const deletedIds = new Set([...prevCachedIds].filter((id) => !currentCachedIds.has(id)));

    prevCachedIdsRef.current = currentCachedIds;

    const newFromCache = (prev: Workflow[]) => {
      const localIds = new Set(prev.map((w) => w.id));
      // Don't add workflows from cache for workspaces where we have temp (pending create) workflows.
      // This prevents races where invalidation resolves before the create API callback.
      const tempWorkspaceIds = new Set(
        prev.filter((w) => w.id.startsWith("temp-")).map((w) => w.workspace_id),
      );
      return visibleCachedWorkflows
        .filter(
          (sw) =>
            !localIds.has(workflowId(sw.id)) &&
            !tempWorkspaceIds.has(toWorkspaceId(sw.workspaceId)),
        )
        .map((sw) => cacheItemToWorkflow(sw));
    };

    setWorkflowItems((prev) => {
      const toAdd = newFromCache(prev);

      // Only remove workflows the store explicitly deleted, keep everything else.
      const filtered = prev.filter((w) => !deletedIds.has(w.id));
      const updated = filtered.map((w) => {
        if (w.id.startsWith("temp-")) return w;
        const sw = visibleCachedWorkflows.find((s) => s.id === w.id);
        if (sw && sw.name !== w.name) return { ...w, name: sw.name };
        return w;
      });

      if (
        toAdd.length === 0 &&
        updated.length === prev.length &&
        updated.every((w, i) => w === prev[i])
      ) {
        return prev;
      }
      return [...toAdd, ...updated];
    });

    setSavedWorkflowItems((prev) => {
      const toAdd = newFromCache(prev);
      const filtered = prev.filter((w) => !deletedIds.has(w.id));
      if (toAdd.length === 0 && filtered.length === prev.length) return prev;
      return [...toAdd, ...filtered];
    });
  }, [visibleCachedWorkflows]);

  const savedWorkflowsById = useMemo(() => {
    return new Map(savedWorkflowItems.map((w) => [w.id, w]));
  }, [savedWorkflowItems]);

  const isWorkflowDirty = (workflow: Workflow) => {
    const saved = savedWorkflowsById.get(workflow.id);
    if (!saved) return true;
    return (
      workflow.name !== saved.name ||
      workflow.description !== saved.description ||
      (workflow.agent_profile_id ?? "") !== (saved.agent_profile_id ?? "")
    );
  };

  return {
    workflowItems,
    setWorkflowItems,
    savedWorkflowItems,
    setSavedWorkflowItems,
    isWorkflowDirty,
  };
}

function isSettingsVisibleWorkflow(workflow: {
  hidden?: boolean;
  style?: "kanban" | "office" | "custom";
}) {
  return !workflow.hidden && workflow.style !== "office";
}

function cacheItemToWorkflow(sw: {
  id: string;
  workspaceId: string;
  name: string;
  description?: string | null;
  style?: Workflow["style"];
}): Workflow {
  return {
    id: workflowId(sw.id),
    workspace_id: toWorkspaceId(sw.workspaceId),
    name: sw.name,
    description: sw.description,
    style: sw.style,
    created_at: "",
    updated_at: "",
  };
}
