"use client";

import { useEffect, useMemo, useRef, useState } from "react";
import { useAppStore } from "@/components/state-provider";
import {
  agentProfileId,
  workflowId,
  workspaceId as toWorkspaceId,
  type Workflow,
} from "@/lib/types/http";

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
  const scopedStoreWorkflows = useScopedStoreWorkflows(storeWorkflows, workspaceId);
  const [workflowItems, setWorkflowItems] = useState<Workflow[]>(initialWorkflows);
  const [savedWorkflowItems, setSavedWorkflowItems] = useState<Workflow[]>(initialWorkflows);
  const savedWorkflowItemsRef = useRef(savedWorkflowItems);
  savedWorkflowItemsRef.current = savedWorkflowItems;

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
        .filter(
          (sw) =>
            !localIds.has(workflowId(sw.id)) &&
            !tempWorkspaceIds.has(toWorkspaceId(sw.workspaceId)),
        )
        .map((sw) => storeItemToWorkflow(sw));
    };

    setWorkflowItems((prev) => {
      const toAdd = newFromStore(prev);

      // Only remove workflows the store explicitly deleted, keep everything else.
      const filtered = prev.filter((w) => !deletedIds.has(w.id));
      const updated = filtered.map((w) => {
        if (w.id.startsWith("temp-")) return w;
        const sw = scopedStoreWorkflows.find((s) => s.id === w.id);
        if (!sw) return w;
        const saved = savedWorkflowItemsRef.current.find((item) => item.id === w.id);
        return mergeDisplayedWorkflowFromStore(w, saved, sw);
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
      const updated = filtered.map((workflow) => {
        const server = scopedStoreWorkflows.find((item) => item.id === workflow.id);
        return server ? mergeSavedWorkflowFromServer(workflow, server) : workflow;
      });
      if (
        toAdd.length === 0 &&
        updated.length === prev.length &&
        updated.every((workflow, index) => workflow === prev[index])
      ) {
        return prev;
      }
      return [...toAdd, ...updated];
    });
  }, [scopedStoreWorkflows]);

  const savedWorkflowsById = useMemo(() => {
    return new Map(savedWorkflowItems.map((w) => [w.id, w]));
  }, [savedWorkflowItems]);

  const isWorkflowDirty = (workflow: Workflow) => workflowIsDirty(workflow, savedWorkflowsById);

  return {
    workflowItems,
    setWorkflowItems,
    savedWorkflowItems,
    setSavedWorkflowItems,
    isWorkflowDirty,
  };
}

function workflowIsDirty(workflow: Workflow, savedWorkflows: Map<string, Workflow>): boolean {
  const saved = savedWorkflows.get(workflow.id);
  if (!saved) return true;
  return (
    workflow.name !== saved.name ||
    (workflow.description ?? "") !== (saved.description ?? "") ||
    (workflow.prompt ?? "") !== (saved.prompt ?? "") ||
    (workflow.agent_profile_id ?? "") !== (saved.agent_profile_id ?? "")
  );
}

type EditableWorkflowField = "name" | "description" | "prompt" | "agent_profile_id";

// Whether the displayed value for `field` has already diverged from the last-known-saved
// baseline — i.e. the user has an uncommitted local edit that a store refresh must not clobber.
// No baseline (a workflow not yet tracked in savedWorkflowItems) is treated as "no draft" so a
// fresh store sync can still populate it, mirroring the pre-existing name-only behavior.
// Normalizes absent values via `?? ""` the same way `workflowIsDirty` does — otherwise a field
// that starts undefined, gets typed into, and is cleared back to "" reads as a phantom draft:
// it blocks a legitimate store sync while the saved baseline still advances underneath it,
// leaving the field falsely dirty and primed to clobber the remote value on the next Save.
function hasLocalDraft(
  displayed: Workflow,
  saved: Workflow | undefined,
  field: EditableWorkflowField,
): boolean {
  return saved != null && (displayed[field] ?? "") !== (saved[field] ?? "");
}

type StoreWorkflowItem = {
  id: string;
  workspaceId: string;
  name: string;
  description?: string | null;
  prompt?: string;
  agent_profile_id?: string;
};

// Pulls fresh values from the store into the displayed/editable draft, field by field, skipping
// any field the user is mid-edit on so a cross-tab update never clobbers an uncommitted change.
function mergeDisplayedWorkflowFromStore(
  displayed: Workflow,
  saved: Workflow | undefined,
  server: StoreWorkflowItem,
): Workflow {
  const next = { ...displayed };
  let changed = false;

  if (!hasLocalDraft(displayed, saved, "name") && server.name !== displayed.name) {
    next.name = server.name;
    changed = true;
  }
  if (
    !hasLocalDraft(displayed, saved, "description") &&
    server.description !== displayed.description
  ) {
    next.description = server.description;
    changed = true;
  }
  if (!hasLocalDraft(displayed, saved, "prompt") && server.prompt !== displayed.prompt) {
    next.prompt = server.prompt;
    changed = true;
  }
  const serverAgentProfileId =
    server.agent_profile_id !== undefined ? agentProfileId(server.agent_profile_id) : undefined;
  if (
    !hasLocalDraft(displayed, saved, "agent_profile_id") &&
    serverAgentProfileId !== displayed.agent_profile_id
  ) {
    next.agent_profile_id = serverAgentProfileId;
    changed = true;
  }

  return changed ? next : displayed;
}

// The dirty-flag baseline always tracks the server's current value for every editable field —
// unlike the displayed draft, it is never guarded by a local-draft check.
function mergeSavedWorkflowFromServer(saved: Workflow, server: StoreWorkflowItem): Workflow {
  const serverAgentProfileId =
    server.agent_profile_id !== undefined ? agentProfileId(server.agent_profile_id) : undefined;
  if (
    server.name === saved.name &&
    server.description === saved.description &&
    server.prompt === saved.prompt &&
    serverAgentProfileId === saved.agent_profile_id
  ) {
    return saved;
  }
  return {
    ...saved,
    name: server.name,
    description: server.description,
    prompt: server.prompt,
    agent_profile_id: serverAgentProfileId,
  };
}

function useScopedStoreWorkflows<
  T extends {
    id: string;
    workspaceId: string;
    name: string;
    description?: string | null;
    prompt?: string;
    agent_profile_id?: string;
    hidden?: boolean;
    style?: string;
  },
>(storeWorkflows: T[], workspaceId?: string): T[] {
  return useMemo(() => {
    const visible = storeWorkflows.filter(
      (workflow) => !workflow.hidden && workflow.style !== "office",
    );
    return workspaceId
      ? visible.filter((workflow) => workflow.workspaceId === workspaceId)
      : visible;
  }, [storeWorkflows, workspaceId]);
}

function storeItemToWorkflow(sw: StoreWorkflowItem): Workflow {
  return {
    id: workflowId(sw.id),
    workspace_id: toWorkspaceId(sw.workspaceId),
    name: sw.name,
    description: sw.description,
    prompt: sw.prompt,
    agent_profile_id:
      sw.agent_profile_id !== undefined ? agentProfileId(sw.agent_profile_id) : undefined,
    created_at: "",
    updated_at: "",
  };
}
