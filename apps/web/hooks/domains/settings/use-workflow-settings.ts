"use client";

import { useEffect, useMemo, useRef, useState } from "react";
import { useAppStore } from "@/components/state-provider";
import type { Workflow } from "@/lib/types/http";

/**
 * Manages workflow list state for the settings page, synced with WS events
 * from the Zustand store. Supports local edits (dirty tracking) and temp drafts.
 *
 * `workspaceId` scopes the visible workflows to the current workspace so that
 * stale entries from previously visited workspaces (still cached in the global
 * Zustand store) don't leak into another workspace's settings page.
 */
export function useWorkflowSettings(initialWorkflows: Workflow[], workspaceId?: string) {
  const storeWorkflows = useAppStore((state) => state.workflows.items);
  const scopedStoreWorkflows = useMemo(
    () =>
      workspaceId ? storeWorkflows.filter((w) => w.workspaceId === workspaceId) : storeWorkflows,
    [storeWorkflows, workspaceId],
  );
  const [workflowItems, setWorkflowItems] = useState<Workflow[]>(initialWorkflows);
  const [savedWorkflowItems, setSavedWorkflowItems] = useState<Workflow[]>(initialWorkflows);

  // Track all IDs we've ever seen from SSR props so we only add genuinely new ones
  // (not re-add workflows the user deleted locally).
  const seenInitialIdsRef = useRef<Set<string>>(new Set(initialWorkflows.map((w) => w.id)));

  // Merge new workflows from SSR props (e.g. after router.refresh() following import).
  // useState ignores updated initialWorkflows on re-render, so we sync manually.
  useEffect(() => {
    const seen = seenInitialIdsRef.current;
    const newWorkflows = initialWorkflows.filter((w) => !seen.has(w.id));
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
  }, [initialWorkflows]);

  // Track which IDs the store has previously reported so we only remove
  // workflows that were actually deleted via WS, not ones the store never knew about.
  const prevStoreIdsRef = useRef<Set<string>>(new Set());

  useEffect(() => {
    const currentStoreIds = new Set(scopedStoreWorkflows.map((w) => w.id));
    const prevStoreIds = prevStoreIdsRef.current;

    // IDs that were in the store last render but are gone now → actually deleted via WS.
    const deletedIds = new Set([...prevStoreIds].filter((id) => !currentStoreIds.has(id)));

    prevStoreIdsRef.current = currentStoreIds;

    const newFromStore = (prev: Workflow[]) => {
      const localIds = new Set(prev.map((w) => w.id));
      // Don't add workflows from store for workspaces where we have temp (pending create) workflows.
      // This prevents race conditions where WS event arrives before the create API callback.
      const tempWorkspaceIds = new Set(
        prev.filter((w) => w.id.startsWith("temp-")).map((w) => w.workspace_id),
      );
      return scopedStoreWorkflows
        .filter((sw) => !localIds.has(sw.id) && !tempWorkspaceIds.has(sw.workspaceId))
        .map((sw) => storeItemToWorkflow(sw));
    };

    setWorkflowItems((prev) => {
      const toAdd = newFromStore(prev);

      // Only remove workflows the store explicitly deleted, keep everything else.
      const filtered = prev.filter((w) => !deletedIds.has(w.id));
      const updated = filtered.map((w) => {
        if (w.id.startsWith("temp-")) return w;
        const sw = scopedStoreWorkflows.find((s) => s.id === w.id);
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
      const toAdd = newFromStore(prev);
      const filtered = prev.filter((w) => !deletedIds.has(w.id));
      if (toAdd.length === 0 && filtered.length === prev.length) return prev;
      return [...toAdd, ...filtered];
    });
  }, [scopedStoreWorkflows]);

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

function storeItemToWorkflow(sw: {
  id: string;
  workspaceId: string;
  name: string;
  description?: string | null;
}): Workflow {
  return {
    id: sw.id,
    workspace_id: sw.workspaceId,
    name: sw.name,
    description: sw.description,
    created_at: "",
    updated_at: "",
  };
}
