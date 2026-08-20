import { act, renderHook } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { useSettingsState } from "./settings-content";
import { updateWorkspaceAction } from "@/app/actions/workspaces";
import { getWorkspaceSettings } from "@/lib/api/domains/office-api";
import type { WorkspaceState } from "@/lib/state/slices/workspace/types";

vi.mock("@/app/actions/workspaces", () => ({
  updateWorkspaceAction: vi.fn(),
}));

vi.mock("@/lib/api/domains/office-api", () => ({
  updateWorkspaceSettings: vi.fn(),
  getWorkspaceSettings: vi.fn(),
}));

const mockUpdateWorkspaceAction = vi.mocked(updateWorkspaceAction);
const mockGetWorkspaceSettings = vi.mocked(getWorkspaceSettings);

type Workspace = WorkspaceState["items"][number];

const WORKSPACE_ID = "ws-1";
const NEW_DESCRIPTION = "New description";

function workspace(overrides: Partial<Workspace> = {}): Workspace {
  return {
    id: WORKSPACE_ID,
    name: "Original Name",
    description: "Original description",
    owner_id: "user-1",
    created_at: "2026-01-01T00:00:00Z",
    updated_at: "2026-01-01T00:00:00Z",
    ...overrides,
  };
}

// Mirrors the slice of `StoreApi<AppState>` the appearance-save handler
// reads: a live `getState()` accessor, not a snapshot passed at render time.
// This is what lets the test below prove a concurrent store write survives.
function makeStoreApi(initialItems: Workspace[], onSetWorkspaces?: (items: Workspace[]) => void) {
  let items = initialItems;
  const setWorkspaces = vi.fn((next: Workspace[]) => {
    items = next;
    onSetWorkspaces?.(next);
  });
  return {
    storeApi: {
      getState: () => ({ workspaces: { items, activeId: items[0]?.id ?? null }, setWorkspaces }),
    },
    setWorkspaces,
    getItems: () => items,
    setItems: (next: Workspace[]) => {
      items = next;
    },
  };
}

function renderSettings(
  activeWorkspace: Workspace,
  storeApi: ReturnType<typeof makeStoreApi>["storeApi"],
) {
  return renderHook(({ ws }: { ws: Workspace }) => useSettingsState(ws, storeApi), {
    initialProps: { ws: activeWorkspace },
  });
}

describe("useSettingsState appearance save", () => {
  beforeEach(() => {
    mockUpdateWorkspaceAction.mockReset();
    mockGetWorkspaceSettings.mockReset();
    mockGetWorkspaceSettings.mockResolvedValue({ settings: {} } as never);
  });

  it("saves the trimmed name and description through updateWorkspaceAction and syncs the store", async () => {
    const ws = workspace();
    const { storeApi, setWorkspaces } = makeStoreApi([ws]);
    mockUpdateWorkspaceAction.mockResolvedValue({
      ...ws,
      name: "Renamed",
      description: NEW_DESCRIPTION,
    } as never);

    const { result } = renderSettings(ws, storeApi);

    act(() => {
      result.current.setName("  Renamed  ");
      result.current.setDescription(NEW_DESCRIPTION);
    });

    await act(async () => {
      await result.current.handleSaveAppearance();
    });

    expect(mockUpdateWorkspaceAction).toHaveBeenCalledWith(WORKSPACE_ID, {
      name: "Renamed",
      description: NEW_DESCRIPTION,
    });
    expect(setWorkspaces).toHaveBeenCalledWith([
      { ...ws, name: "Renamed", description: NEW_DESCRIPTION },
    ]);
  });

  it("clears the dirty flag once the store reflects the saved workspace", async () => {
    const ws = workspace();
    const { storeApi, getItems } = makeStoreApi([ws]);
    mockUpdateWorkspaceAction.mockResolvedValue({ ...ws, name: "Renamed" } as never);

    const { result, rerender } = renderSettings(ws, storeApi);

    act(() => result.current.setName("Renamed"));
    expect(result.current.appearanceDirty).toBe(true);

    await act(async () => {
      await result.current.handleSaveAppearance();
    });

    // Simulate the Zustand store propagating the update back into
    // `activeWorkspace`, the way SettingsContent's subscription would on the
    // next render.
    rerender({ ws: getItems()[0] });

    expect(result.current.appearanceDirty).toBe(false);
  });

  it("refuses to save an all-whitespace name", async () => {
    const ws = workspace();
    const { storeApi, setWorkspaces } = makeStoreApi([ws]);
    const { result } = renderSettings(ws, storeApi);

    act(() => result.current.setName("   "));

    await act(async () => {
      await result.current.handleSaveAppearance();
    });

    expect(mockUpdateWorkspaceAction).not.toHaveBeenCalled();
    expect(setWorkspaces).not.toHaveBeenCalled();
  });

  // A newer keystroke landing before the PATCH from an older one resolves
  // must survive that response, not get overwritten by the stale echo.
  it("keeps a draft typed during an in-flight save instead of reverting it", async () => {
    const ws = workspace();
    const { storeApi } = makeStoreApi([ws]);
    let resolveUpdate!: (value: Workspace) => void;
    mockUpdateWorkspaceAction.mockImplementation(
      () => new Promise<Workspace>((resolve) => (resolveUpdate = resolve)) as never,
    );

    const { result } = renderSettings(ws, storeApi);
    act(() => result.current.setName("Alpha"));
    let savePromise!: Promise<void>;
    act(() => {
      savePromise = result.current.handleSaveAppearance();
    });
    act(() => result.current.setName("Alpha2"));

    await act(async () => {
      resolveUpdate({ ...ws, name: "Alpha" });
      await savePromise;
    });

    expect(result.current.name).toBe("Alpha2");
  });

  it("does not clobber a workspace added to the store while the save was in flight", async () => {
    const ws = workspace();
    const concurrent = workspace({ id: "ws-2", name: "Concurrent Workspace" });
    const { storeApi, setWorkspaces, setItems } = makeStoreApi([ws]);

    // Simulate a `workspace.created` WS event landing mid-PATCH: the store
    // gains a second workspace between render (when the old code captured
    // its snapshot) and the moment the save handler actually reads state.
    mockUpdateWorkspaceAction.mockImplementation(async () => {
      setItems([ws, concurrent]);
      return { ...ws, name: "Renamed", description: NEW_DESCRIPTION } as never;
    });

    const { result } = renderSettings(ws, storeApi);

    act(() => {
      result.current.setName("Renamed");
      result.current.setDescription(NEW_DESCRIPTION);
    });

    await act(async () => {
      await result.current.handleSaveAppearance();
    });

    expect(setWorkspaces).toHaveBeenCalledWith([
      { ...ws, name: "Renamed", description: NEW_DESCRIPTION },
      concurrent,
    ]);
  });
});
