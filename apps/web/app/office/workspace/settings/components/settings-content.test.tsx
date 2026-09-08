import { act, cleanup, fireEvent, renderHook, screen } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { useSettingsState } from "./settings-content";
import { updateWorkspaceAction } from "@/app/actions/workspaces";
import { getWorkspaceSettings } from "@/lib/api/domains/office-api";
import type { WorkspaceState } from "@/lib/state/slices/workspace/types";
import {
  SettingsSaveProvider,
  useSettingsSaveCoordinator,
} from "@/components/settings/settings-save-provider";

vi.mock("@/app/actions/workspaces", () => ({
  updateWorkspaceAction: vi.fn(),
}));

vi.mock("@/lib/api/domains/office-api", async (importOriginal) => {
  const actual = await importOriginal<typeof import("@/lib/api/domains/office-api")>();
  return { ...actual, updateWorkspaceSettings: vi.fn(), getWorkspaceSettings: vi.fn() };
});

const mockUpdateWorkspaceAction = vi.mocked(updateWorkspaceAction);
const mockGetWorkspaceSettings = vi.mocked(getWorkspaceSettings);

type Workspace = WorkspaceState["items"][number];

const WORKSPACE_ID = "ws-1";
const NEW_DESCRIPTION = "New description";
const REMOTE_NAME = "Remote Name";
const REMOTE_DESCRIPTION = "Remote description";

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
    wrapper: SettingsSaveProvider,
  });
}

function renderCoordinatedSettings(
  activeWorkspace: Workspace,
  storeApi: ReturnType<typeof makeStoreApi>["storeApi"],
) {
  return renderHook(
    ({ ws }: { ws: Workspace }) => {
      const settings = useSettingsState(ws, storeApi);
      const coordinator = useSettingsSaveCoordinator();
      return { settings, coordinator };
    },
    { initialProps: { ws: activeWorkspace }, wrapper: SettingsSaveProvider },
  );
}

// Combines the two setup calls every test needs: a store double plus the
// hook rendered against it.
function renderWithStore(items: Workspace[], onSetWorkspaces?: (items: Workspace[]) => void) {
  const store = makeStoreApi(items, onSetWorkspaces);
  return { ...store, ...renderSettings(items[0], store.storeApi) };
}

async function saveAppearance(result: ReturnType<typeof renderSettings>["result"]) {
  await act(async () => {
    await result.current.handleSaveAppearance();
  });
}

beforeEach(() => {
  vi.resetAllMocks();
  mockGetWorkspaceSettings.mockResolvedValue({ settings: {} } as never);
});

afterEach(cleanup);

describe("useSettingsState appearance save", () => {
  it("saves the trimmed name and description through updateWorkspaceAction and syncs the store", async () => {
    const ws = workspace();
    const { setWorkspaces, result, rerender } = renderWithStore([ws]);
    mockUpdateWorkspaceAction.mockResolvedValue({
      ...ws,
      name: "Renamed",
      description: NEW_DESCRIPTION,
    } as never);

    act(() => {
      result.current.setName("  Renamed  ");
      result.current.setDescription(NEW_DESCRIPTION);
    });

    await saveAppearance(result);

    expect(mockUpdateWorkspaceAction).toHaveBeenCalledWith(WORKSPACE_ID, {
      name: "Renamed",
      description: NEW_DESCRIPTION,
    });
    expect(setWorkspaces).toHaveBeenCalledWith([
      { ...ws, name: "Renamed", description: NEW_DESCRIPTION },
    ]);

    // The submitted draft had leading/trailing whitespace; the echo guard
    // must compare against that raw draft, not its trimmed form, or the
    // input keeps the whitespace forever and the dirty flag never clears.
    rerender({ ws: { ...ws, name: "Renamed", description: NEW_DESCRIPTION } });
    expect(result.current.name).toBe("Renamed");
    expect(result.current.appearanceDirty).toBe(false);
  });

  it("clears the dirty flag once the store reflects the saved workspace", async () => {
    const { getItems, result, rerender } = renderWithStore([workspace()]);
    mockUpdateWorkspaceAction.mockResolvedValue({ ...workspace(), name: "Renamed" } as never);

    act(() => result.current.setName("Renamed"));
    expect(result.current.appearanceDirty).toBe(true);

    await saveAppearance(result);

    // Simulate the Zustand store propagating the update back into
    // `activeWorkspace`, the way SettingsContent's subscription would on the
    // next render.
    rerender({ ws: getItems()[0] });

    expect(result.current.appearanceDirty).toBe(false);
  });

  it("refuses to save an all-whitespace name", async () => {
    const { setWorkspaces, result } = renderWithStore([workspace()]);

    act(() => result.current.setName("   "));

    await saveAppearance(result);

    expect(mockUpdateWorkspaceAction).not.toHaveBeenCalled();
    expect(setWorkspaces).not.toHaveBeenCalled();
  });

  // A newer keystroke landing before the PATCH from an older one resolves
  // must survive that response, not get overwritten by the stale echo.
  it("keeps a draft typed during an in-flight save instead of reverting it", async () => {
    const ws = workspace();
    let resolveUpdate!: (value: Workspace) => void;
    mockUpdateWorkspaceAction.mockImplementation(
      () => new Promise<Workspace>((resolve) => (resolveUpdate = resolve)) as never,
    );

    const { result } = renderWithStore([ws]);
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

  // A workspace.deleted WS event elsewhere can reassign the active workspace
  // without this page remounting; the draft must reset to the new one's values.
  it("resets the draft when the active workspace's identity changes", async () => {
    const { result, rerender } = renderWithStore([workspace()]);
    act(() => result.current.setName("Unsaved Draft"));
    expect(result.current.appearanceDirty).toBe(true);
    rerender({ ws: workspace({ id: "ws-2", name: "Other Workspace", description: "Other desc" }) });
    expect(result.current.name).toBe("Other Workspace");
    expect(result.current.description).toBe("Other desc");
    expect(result.current.appearanceDirty).toBe(false);
  });

  it("does not clobber a workspace added to the store while the save was in flight", async () => {
    const ws = workspace();
    const concurrent = workspace({ id: "ws-2", name: "Concurrent Workspace" });
    const { setWorkspaces, setItems, result } = renderWithStore([ws]);

    // Simulate a `workspace.created` WS event landing mid-PATCH: the store
    // gains a second workspace between render (when the old code captured
    // its snapshot) and the moment the save handler actually reads state.
    mockUpdateWorkspaceAction.mockImplementation(async () => {
      setItems([ws, concurrent]);
      return { ...ws, name: "Renamed", description: NEW_DESCRIPTION } as never;
    });

    act(() => {
      result.current.setName("Renamed");
      result.current.setDescription(NEW_DESCRIPTION);
    });

    await saveAppearance(result);

    expect(setWorkspaces).toHaveBeenCalledWith([
      { ...ws, name: "Renamed", description: NEW_DESCRIPTION },
      concurrent,
    ]);
  });
});

describe("coordinated appearance save", () => {
  it("registers appearance drafts with the shared save coordinator", async () => {
    const ws = workspace();
    const store = makeStoreApi([ws]);
    const { result } = renderCoordinatedSettings(ws, store.storeApi);
    mockUpdateWorkspaceAction.mockResolvedValue({ ...ws, name: "Renamed" } as never);

    act(() => result.current.settings.setName("Renamed"));

    expect(result.current.coordinator.hasDirty).toBe(true);
    await act(async () => {
      await expect(result.current.coordinator.saveAll()).resolves.toMatchObject({
        canLeave: true,
      });
    });

    expect(mockUpdateWorkspaceAction).toHaveBeenCalledWith(WORKSPACE_ID, {
      name: "Renamed",
      description: ws.description,
    });
    expect(store.setWorkspaces).toHaveBeenCalled();
  });

  it("propagates save failures after clearing saving state", async () => {
    mockUpdateWorkspaceAction.mockRejectedValue(new Error("network error"));
    const { setWorkspaces, result } = renderWithStore([workspace()]);

    act(() => result.current.setName("Changed"));
    await expect(
      act(async () => {
        await result.current.handleSaveAppearance();
      }),
    ).rejects.toThrow("network error");

    expect(setWorkspaces).not.toHaveBeenCalled();
    expect(result.current.savingAppearance).toBe(false);
  });

  it("resets the appearance draft to the authoritative workspace baseline", async () => {
    const ws = workspace();
    const { result } = renderCoordinatedSettings(ws, makeStoreApi([ws]).storeApi);

    act(() => {
      result.current.settings.setName("Unsaved name");
      result.current.settings.setDescription("Unsaved description");
    });

    expect(screen.getByTestId("settings-floating-save")).toBeTruthy();
    await act(async () => {
      fireEvent.click(screen.getByRole("button", { name: /reset/i }));
    });

    expect(result.current.settings.name).toBe(ws.name);
    expect(result.current.settings.description).toBe(ws.description);
    expect(result.current.settings.appearanceDirty).toBe(false);
  });
});

describe("same-workspace appearance updates", () => {
  it("syncs an untouched draft when the same workspace receives a remote update", () => {
    const ws = workspace();
    const { result, rerender } = renderWithStore([ws]);

    rerender({ ws: { ...ws, name: REMOTE_NAME, description: REMOTE_DESCRIPTION } });

    expect(result.current.name).toBe(REMOTE_NAME);
    expect(result.current.description).toBe(REMOTE_DESCRIPTION);
    expect(result.current.appearanceDirty).toBe(false);
  });

  it("preserves edited fields while syncing untouched fields from a remote update", () => {
    const ws = workspace();
    const { result, rerender } = renderWithStore([ws]);
    act(() => result.current.setName("Local Draft"));

    rerender({ ws: { ...ws, name: REMOTE_NAME, description: REMOTE_DESCRIPTION } });

    expect(result.current.name).toBe("Local Draft");
    expect(result.current.description).toBe(REMOTE_DESCRIPTION);
    expect(result.current.appearanceDirty).toBe(true);
  });
});
