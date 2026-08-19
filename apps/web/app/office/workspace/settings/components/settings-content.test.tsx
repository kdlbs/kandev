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

function renderSettings(
  activeWorkspace: Workspace,
  workspaces: Workspace[],
  setWorkspaces: (items: Workspace[]) => void,
) {
  return renderHook(
    ({ ws, list }: { ws: Workspace; list: Workspace[] }) =>
      useSettingsState(ws, list, setWorkspaces),
    { initialProps: { ws: activeWorkspace, list: workspaces } },
  );
}

describe("useSettingsState appearance save", () => {
  beforeEach(() => {
    mockUpdateWorkspaceAction.mockReset();
    mockGetWorkspaceSettings.mockReset();
    mockGetWorkspaceSettings.mockResolvedValue({ settings: {} } as never);
  });

  it("saves the trimmed name and description through updateWorkspaceAction and syncs the store", async () => {
    const ws = workspace();
    const setWorkspaces = vi.fn();
    mockUpdateWorkspaceAction.mockResolvedValue({
      ...ws,
      name: "Renamed",
      description: NEW_DESCRIPTION,
    } as never);

    const { result } = renderSettings(ws, [ws], setWorkspaces);

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
    let stored: Workspace[] = [ws];
    const setWorkspaces = vi.fn((items: Workspace[]) => {
      stored = items;
    });
    mockUpdateWorkspaceAction.mockResolvedValue({ ...ws, name: "Renamed" } as never);

    const { result, rerender } = renderSettings(ws, [ws], setWorkspaces);

    act(() => result.current.setName("Renamed"));
    expect(result.current.appearanceDirty).toBe(true);

    await act(async () => {
      await result.current.handleSaveAppearance();
    });

    // Simulate the Zustand store propagating the update back into
    // `activeWorkspace`, the way SettingsContent's subscription would on the
    // next render.
    rerender({ ws: stored[0], list: stored });

    expect(result.current.appearanceDirty).toBe(false);
  });

  it("refuses to save an all-whitespace name", async () => {
    const ws = workspace();
    const setWorkspaces = vi.fn();
    const { result } = renderSettings(ws, [ws], setWorkspaces);

    act(() => result.current.setName("   "));

    await act(async () => {
      await result.current.handleSaveAppearance();
    });

    expect(mockUpdateWorkspaceAction).not.toHaveBeenCalled();
    expect(setWorkspaces).not.toHaveBeenCalled();
  });
});
