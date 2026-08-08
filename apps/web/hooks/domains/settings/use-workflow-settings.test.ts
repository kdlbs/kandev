import { describe, it, expect, vi, beforeEach } from "vitest";
import { renderHook, act } from "@testing-library/react";
import {
  agentProfileId,
  workflowId as toWorkflowId,
  workspaceId as toWorkspaceId,
  type Workflow,
} from "@/lib/types/http";

type StoreWorkflow = {
  id: string;
  workspaceId: string;
  name: string;
  description?: string | null;
  prompt?: string;
  agent_profile_id?: string;
  hidden?: boolean;
  style?: "kanban" | "office" | "custom";
};

type MockState = { workflows: { items: StoreWorkflow[] } };

let mockState: MockState = { workflows: { items: [] } };

vi.mock("@/components/state-provider", () => ({
  useAppStore: (selector: (s: MockState) => unknown) => selector(mockState),
}));

import { useWorkflowSettings } from "./use-workflow-settings";

function setStore(items: StoreWorkflow[]) {
  mockState = { workflows: { items } };
}

const wf = (id: string, wsId: string, name: string): Workflow => ({
  id: toWorkflowId(id),
  workspace_id: toWorkspaceId(wsId),
  name,
  description: "",
  created_at: "",
  updated_at: "",
});

const NAME_A1 = "Workflow A1";
const NAME_B1 = "Workflow B1";
const STORE_A1: StoreWorkflow = { id: "wf-a1", workspaceId: "ws-a", name: NAME_A1 };
const STORE_B1: StoreWorkflow = { id: "wf-b1", workspaceId: "ws-b", name: NAME_B1 };

const OLD_DESCRIPTION = "Old description";
const NEW_DESCRIPTION = "New description";
const OLD_PROMPT = "Old prompt";
const NEW_PROMPT = "New prompt";
const UNSAVED_LOCAL_DRAFT = "Unsaved local draft";

// Shared by the cross-tab field-sync tests below: renders the hook with a
// rerender-able store snapshot so a test can simulate a WS-driven store update.
function renderWithStoreItem(initial: Workflow[], store: StoreWorkflow) {
  return renderHook(
    ({ storeItems }: { storeItems: StoreWorkflow[] }) => {
      setStore(storeItems);
      return useWorkflowSettings(initial, "ws-b");
    },
    { initialProps: { storeItems: [store] } },
  );
}

beforeEach(() => {
  setStore([]);
});

describe("useWorkflowSettings — store boundary filters", () => {
  it("does not merge office-style workflows from the global store", () => {
    // The sidebar's `useEnsureWorkspaceWorkflows` populates the store with every
    // workflow — including office-style ones — on all routes. The settings UI
    // is kanban-only (ADR-0004), so office workflows must be dropped at the
    // store boundary (matching the SSR-side filter). Otherwise they land in
    // `workflowItems`, which is what "Export All" serialises → workflow-import-
    // export e2e regresses.
    const officeInB: StoreWorkflow = {
      id: "wf-b-office",
      workspaceId: "ws-b",
      name: "Office Only Workflow",
      style: "office",
    };
    setStore([officeInB]);

    const { result } = renderHook(() => useWorkflowSettings([], "ws-b"));

    expect(result.current.workflowItems).toHaveLength(0);
    expect(result.current.savedWorkflowItems).toHaveLength(0);
  });
});

describe("useWorkflowSettings", () => {
  it("does not include workflows from other workspaces present in the global store", () => {
    // Store has a workflow from workspace A (e.g. user previously visited it)
    setStore([STORE_A1]);

    // We render the settings hook for workspace B with no initial workflows
    const { result } = renderHook(() => useWorkflowSettings([], "ws-b"));

    // The leaked workflow from workspace A must not appear in B's list
    expect(result.current.workflowItems).toHaveLength(0);
    expect(result.current.savedWorkflowItems).toHaveLength(0);
  });

  it("adds workflows from the store that belong to the current workspace", () => {
    setStore([STORE_A1, STORE_B1]);

    const { result } = renderHook(() => useWorkflowSettings([], "ws-b"));

    expect(result.current.workflowItems.map((w) => w.id)).toEqual(["wf-b1"]);
  });

  it("does not remove a workspace's workflows when an unrelated workspace's entries are added/removed in the store", () => {
    // Initial: workspace B has one saved workflow from SSR
    const initial = [wf("wf-b1", "ws-b", NAME_B1)];
    setStore([STORE_B1]);

    const { result, rerender } = renderHook(
      ({ store }: { store: StoreWorkflow[] }) => {
        setStore(store);
        return useWorkflowSettings(initial, "ws-b");
      },
      { initialProps: { store: [STORE_B1] } },
    );

    expect(result.current.workflowItems.map((w) => w.id)).toEqual(["wf-b1"]);

    // Workspace A workflow is added to the store (e.g. WS event from another tab)
    act(() => {
      rerender({ store: [STORE_B1, STORE_A1] });
    });
    expect(result.current.workflowItems.map((w) => w.id)).toEqual(["wf-b1"]);

    // Workspace A workflow is removed from the store — must not affect B's list
    act(() => {
      rerender({ store: [STORE_B1] });
    });
    expect(result.current.workflowItems.map((w) => w.id)).toEqual(["wf-b1"]);
  });

  it("falls back to the unscoped store when no workspaceId is provided", () => {
    setStore([STORE_A1, STORE_B1]);

    const { result } = renderHook(() => useWorkflowSettings([]));

    expect(result.current.workflowItems.map((w) => w.id).sort()).toEqual(["wf-a1", "wf-b1"]);
  });

  it("syncs name updates from the store within the current workspace", () => {
    const initial = [wf("wf-b1", "ws-b", NAME_B1)];
    setStore([STORE_B1]);

    const { result, rerender } = renderHook(
      ({ store }: { store: StoreWorkflow[] }) => {
        setStore(store);
        return useWorkflowSettings(initial, "ws-b");
      },
      { initialProps: { store: [STORE_B1] } },
    );

    expect(result.current.workflowItems[0].name).toEqual(NAME_B1);

    act(() => {
      rerender({ store: [{ id: "wf-b1", workspaceId: "ws-b", name: "Renamed B1" }] });
    });

    expect(result.current.workflowItems[0].name).toEqual("Renamed B1");
  });

  it("does not overwrite a dirty name when the store refreshes", () => {
    const initial = [wf("wf-b1", "ws-b", NAME_B1)];
    const { result, rerender } = renderHook(
      ({ store }: { store: StoreWorkflow[] }) => {
        setStore(store);
        return useWorkflowSettings(initial, "ws-b");
      },
      { initialProps: { store: [STORE_B1] } },
    );

    act(() => {
      result.current.setWorkflowItems((items) =>
        items.map((item) => ({ ...item, name: "Unsaved local name" })),
      );
    });
    act(() => {
      rerender({ store: [{ ...STORE_B1, name: "Remote name" }] });
    });

    expect(result.current.workflowItems[0].name).toBe("Unsaved local name");
    expect(result.current.savedWorkflowItems[0].name).toBe("Remote name");
    expect(result.current.isWorkflowDirty(result.current.workflowItems[0])).toBe(true);
  });

  it("does not report a workflow dirty after its description is edited then cleared back to an absent saved description", () => {
    // Backend omits `description` entirely when empty (omitempty), so a
    // workflow with no saved description has `saved.description === undefined`,
    // not "". Typing into the description field and then clearing it sets the
    // draft to "" — that must still compare equal to the absent saved value.
    const initial: Workflow[] = [{ ...wf("wf-b1", "ws-b", NAME_B1), description: undefined }];
    const { result } = renderHook(() => useWorkflowSettings(initial, "ws-b"));

    act(() => {
      result.current.setWorkflowItems((items) =>
        items.map((item) => ({ ...item, description: "test description" })),
      );
    });
    expect(result.current.isWorkflowDirty(result.current.workflowItems[0])).toBe(true);

    act(() => {
      result.current.setWorkflowItems((items) =>
        items.map((item) => ({ ...item, description: "" })),
      );
    });
    expect(result.current.isWorkflowDirty(result.current.workflowItems[0])).toBe(false);
  });
});

describe("useWorkflowSettings — cross-tab field sync", () => {
  it("syncs a description update from the store within the current workspace", () => {
    const initial = [{ ...wf("wf-b1", "ws-b", NAME_B1), description: OLD_DESCRIPTION }];
    const store: StoreWorkflow = { ...STORE_B1, description: OLD_DESCRIPTION };
    const { result, rerender } = renderWithStoreItem(initial, store);

    act(() => {
      rerender({ storeItems: [{ ...store, description: NEW_DESCRIPTION }] });
    });

    expect(result.current.workflowItems[0].description).toBe(NEW_DESCRIPTION);
    expect(result.current.savedWorkflowItems[0].description).toBe(NEW_DESCRIPTION);
  });

  it("does not overwrite a dirty description when the store refreshes, but still moves the dirty-flag baseline", () => {
    const initial = [{ ...wf("wf-b1", "ws-b", NAME_B1), description: OLD_DESCRIPTION }];
    const store: StoreWorkflow = { ...STORE_B1, description: OLD_DESCRIPTION };
    const { result, rerender } = renderWithStoreItem(initial, store);

    act(() => {
      result.current.setWorkflowItems((items) =>
        items.map((item) => ({ ...item, description: UNSAVED_LOCAL_DRAFT })),
      );
    });
    act(() => {
      rerender({ storeItems: [{ ...store, description: NEW_DESCRIPTION }] });
    });

    expect(result.current.workflowItems[0].description).toBe(UNSAVED_LOCAL_DRAFT);
    expect(result.current.savedWorkflowItems[0].description).toBe(NEW_DESCRIPTION);
    expect(result.current.isWorkflowDirty(result.current.workflowItems[0])).toBe(true);
  });

  it("syncs a prompt update from the store within the current workspace", () => {
    const initial = [{ ...wf("wf-b1", "ws-b", NAME_B1), prompt: OLD_PROMPT }];
    const store: StoreWorkflow = { ...STORE_B1, prompt: OLD_PROMPT };
    const { result, rerender } = renderWithStoreItem(initial, store);

    act(() => {
      rerender({ storeItems: [{ ...store, prompt: NEW_PROMPT }] });
    });

    expect(result.current.workflowItems[0].prompt).toBe(NEW_PROMPT);
    expect(result.current.savedWorkflowItems[0].prompt).toBe(NEW_PROMPT);
  });

  it("does not overwrite a dirty prompt when the store refreshes, but still moves the dirty-flag baseline", () => {
    const initial = [{ ...wf("wf-b1", "ws-b", NAME_B1), prompt: OLD_PROMPT }];
    const store: StoreWorkflow = { ...STORE_B1, prompt: OLD_PROMPT };
    const { result, rerender } = renderWithStoreItem(initial, store);

    act(() => {
      result.current.setWorkflowItems((items) =>
        items.map((item) => ({ ...item, prompt: UNSAVED_LOCAL_DRAFT })),
      );
    });
    act(() => {
      rerender({ storeItems: [{ ...store, prompt: NEW_PROMPT }] });
    });

    expect(result.current.workflowItems[0].prompt).toBe(UNSAVED_LOCAL_DRAFT);
    expect(result.current.savedWorkflowItems[0].prompt).toBe(NEW_PROMPT);
    expect(result.current.isWorkflowDirty(result.current.workflowItems[0])).toBe(true);
  });

  it("syncs an agent_profile_id update from the store within the current workspace", () => {
    const initial = [
      { ...wf("wf-b1", "ws-b", NAME_B1), agent_profile_id: agentProfileId("agent-old") },
    ];
    const store: StoreWorkflow = { ...STORE_B1, agent_profile_id: "agent-old" };
    const { result, rerender } = renderWithStoreItem(initial, store);

    act(() => {
      rerender({ storeItems: [{ ...store, agent_profile_id: "agent-new" }] });
    });

    expect(result.current.workflowItems[0].agent_profile_id).toBe(agentProfileId("agent-new"));
    expect(result.current.savedWorkflowItems[0].agent_profile_id).toBe(agentProfileId("agent-new"));
  });

  it("does not overwrite a dirty agent_profile_id when the store refreshes, but still moves the dirty-flag baseline", () => {
    const initial = [
      { ...wf("wf-b1", "ws-b", NAME_B1), agent_profile_id: agentProfileId("agent-old") },
    ];
    const store: StoreWorkflow = { ...STORE_B1, agent_profile_id: "agent-old" };
    const { result, rerender } = renderWithStoreItem(initial, store);

    act(() => {
      result.current.setWorkflowItems((items) =>
        items.map((item) => ({ ...item, agent_profile_id: agentProfileId("agent-draft") })),
      );
    });
    act(() => {
      rerender({ storeItems: [{ ...store, agent_profile_id: "agent-new" }] });
    });

    expect(result.current.workflowItems[0].agent_profile_id).toBe(agentProfileId("agent-draft"));
    expect(result.current.savedWorkflowItems[0].agent_profile_id).toBe(agentProfileId("agent-new"));
    expect(result.current.isWorkflowDirty(result.current.workflowItems[0])).toBe(true);
  });
});

// Regression coverage: a field that starts absent (undefined), gets typed into and
// cleared back to "" is not dirty per `isWorkflowDirty` (which normalizes via `?? ""`),
// so a remote update landing afterward must still sync — not get stuck behind a
// false-positive "local draft" that also silently drags the dirty-flag baseline out
// from under the stuck display, priming the next Save to clobber the remote value.
describe("useWorkflowSettings — cross-tab field sync (absent-vs-empty normalization)", () => {
  it("syncs a description update after a local edit is cleared back to the absent saved value", () => {
    const initial = [{ ...wf("wf-b1", "ws-b", NAME_B1), description: undefined }];
    const store: StoreWorkflow = { ...STORE_B1 };
    const { result, rerender } = renderWithStoreItem(initial, store);

    act(() => {
      result.current.setWorkflowItems((items) =>
        items.map((item) => ({ ...item, description: "typed then cleared" })),
      );
    });
    act(() => {
      result.current.setWorkflowItems((items) =>
        items.map((item) => ({ ...item, description: "" })),
      );
    });
    expect(result.current.isWorkflowDirty(result.current.workflowItems[0])).toBe(false);

    act(() => {
      rerender({ storeItems: [{ ...store, description: NEW_DESCRIPTION }] });
    });

    expect(result.current.workflowItems[0].description).toBe(NEW_DESCRIPTION);
    expect(result.current.savedWorkflowItems[0].description).toBe(NEW_DESCRIPTION);
    expect(result.current.isWorkflowDirty(result.current.workflowItems[0])).toBe(false);
  });

  it("syncs a prompt update after a local edit is cleared back to the absent saved value", () => {
    const initial = [{ ...wf("wf-b1", "ws-b", NAME_B1), prompt: undefined }];
    const store: StoreWorkflow = { ...STORE_B1 };
    const { result, rerender } = renderWithStoreItem(initial, store);

    act(() => {
      result.current.setWorkflowItems((items) =>
        items.map((item) => ({ ...item, prompt: "typed then cleared" })),
      );
    });
    act(() => {
      result.current.setWorkflowItems((items) => items.map((item) => ({ ...item, prompt: "" })));
    });
    expect(result.current.isWorkflowDirty(result.current.workflowItems[0])).toBe(false);

    act(() => {
      rerender({ storeItems: [{ ...store, prompt: NEW_PROMPT }] });
    });

    expect(result.current.workflowItems[0].prompt).toBe(NEW_PROMPT);
    expect(result.current.savedWorkflowItems[0].prompt).toBe(NEW_PROMPT);
    expect(result.current.isWorkflowDirty(result.current.workflowItems[0])).toBe(false);
  });

  it("syncs an agent_profile_id update after a local edit is cleared back to the absent saved value", () => {
    // Mirrors the UI: selecting "None" in the agent profile picker sets
    // agent_profile_id to "" (workflow-card.tsx), not undefined.
    const initial = [{ ...wf("wf-b1", "ws-b", NAME_B1), agent_profile_id: undefined }];
    const store: StoreWorkflow = { ...STORE_B1 };
    const { result, rerender } = renderWithStoreItem(initial, store);

    act(() => {
      result.current.setWorkflowItems((items) =>
        items.map((item) => ({ ...item, agent_profile_id: agentProfileId("agent-draft") })),
      );
    });
    act(() => {
      result.current.setWorkflowItems((items) =>
        items.map((item) => ({ ...item, agent_profile_id: agentProfileId("") })),
      );
    });
    expect(result.current.isWorkflowDirty(result.current.workflowItems[0])).toBe(false);

    act(() => {
      rerender({ storeItems: [{ ...store, agent_profile_id: "agent-new" }] });
    });

    expect(result.current.workflowItems[0].agent_profile_id).toBe(agentProfileId("agent-new"));
    expect(result.current.savedWorkflowItems[0].agent_profile_id).toBe(agentProfileId("agent-new"));
    expect(result.current.isWorkflowDirty(result.current.workflowItems[0])).toBe(false);
  });

  it("does not sync a description update while a different field (name) has a genuine local draft", () => {
    const initial = [{ ...wf("wf-b1", "ws-b", NAME_B1), description: OLD_DESCRIPTION }];
    const store: StoreWorkflow = { ...STORE_B1, description: OLD_DESCRIPTION };
    const { result, rerender } = renderWithStoreItem(initial, store);

    act(() => {
      result.current.setWorkflowItems((items) =>
        items.map((item) => ({ ...item, name: UNSAVED_LOCAL_DRAFT })),
      );
    });
    act(() => {
      rerender({ storeItems: [{ ...store, description: NEW_DESCRIPTION }] });
    });

    // The name draft is preserved, and the unrelated description field still syncs live.
    expect(result.current.workflowItems[0].name).toBe(UNSAVED_LOCAL_DRAFT);
    expect(result.current.workflowItems[0].description).toBe(NEW_DESCRIPTION);
    expect(result.current.savedWorkflowItems[0].description).toBe(NEW_DESCRIPTION);
    expect(result.current.isWorkflowDirty(result.current.workflowItems[0])).toBe(true);
  });
});

describe("useWorkflowSettings visibility", () => {
  it("excludes hidden system workflows from the settings list", () => {
    // System workflows like "Improve Kandev" live in the global store with
    // hidden=true so the kanban can resolve task references, but they must
    // never appear in the management UI.
    const HIDDEN_SYSTEM: StoreWorkflow = {
      id: "wf-improve-kandev",
      workspaceId: "ws-b",
      name: "Improve Kandev",
      hidden: true,
    };
    setStore([STORE_B1, HIDDEN_SYSTEM]);

    const { result } = renderHook(() => useWorkflowSettings([], "ws-b"));

    expect(result.current.workflowItems.map((w) => w.id)).toEqual(["wf-b1"]);
    expect(result.current.savedWorkflowItems.map((w) => w.id)).toEqual(["wf-b1"]);
  });

  it("drops a workflow from the settings list once it becomes hidden", () => {
    const initial = [wf("wf-b1", "ws-b", NAME_B1)];
    setStore([STORE_B1]);

    const { result, rerender } = renderHook(
      ({ store }: { store: StoreWorkflow[] }) => {
        setStore(store);
        return useWorkflowSettings(initial, "ws-b");
      },
      { initialProps: { store: [STORE_B1] } },
    );

    expect(result.current.workflowItems.map((w) => w.id)).toEqual(["wf-b1"]);

    // Backend flips hidden=true (e.g. healing the improve-kandev record).
    act(() => {
      rerender({ store: [{ ...STORE_B1, hidden: true }] });
    });

    expect(result.current.workflowItems.map((w) => w.id)).toEqual([]);
  });

  it("starts scoping store entries once a workspaceId becomes defined", () => {
    setStore([STORE_A1, STORE_B1]);

    const { result, rerender } = renderHook(
      ({ workspaceId }: { workspaceId?: string }) => useWorkflowSettings([], workspaceId),
      { initialProps: { workspaceId: undefined as string | undefined } },
    );

    // No workspaceId → unscoped fallback shows both
    expect(result.current.workflowItems.map((w) => w.id).sort()).toEqual(["wf-a1", "wf-b1"]);

    act(() => {
      rerender({ workspaceId: "ws-b" });
    });

    // Once scoped to B, A's workflow is dropped
    expect(result.current.workflowItems.map((w) => w.id)).toEqual(["wf-b1"]);
  });
});
