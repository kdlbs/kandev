import type { ReactNode } from "react";
import { renderHook, waitFor } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";
import type { StoreApi } from "zustand";
import { StateProvider, useAppStoreApi } from "@/components/state-provider";
import type { AppState } from "@/lib/state/store";

vi.mock("@/lib/config", () => ({
  getBackendConfig: () => ({ apiBaseUrl: "http://localhost:8443" }),
}));

const listWorkspacesMock = vi.fn();
const fetchUserSettingsMock = vi.fn();
const getOnboardingStateMock = vi.fn();

vi.mock("@/lib/api", () => ({
  listWorkspaces: (...args: unknown[]) => listWorkspacesMock(...args),
  fetchUserSettings: (...args: unknown[]) => fetchUserSettingsMock(...args),
}));

vi.mock("@/lib/api/domains/office-api", () => ({
  getOnboardingState: (...args: unknown[]) => getOnboardingStateMock(...args),
}));

import { useOfficeRouteBootstrap } from "./office-routes";

const OFFICE_WORKSPACE_ID = "ws-office-1";
const OFFICE_WORKSPACES_RESPONSE = {
  workspaces: [
    {
      id: OFFICE_WORKSPACE_ID,
      name: "Office workspace",
      owner_id: "user-1",
      office_workflow_id: "office-flow-1",
      created_at: "2026-01-01T00:00:00Z",
      updated_at: "2026-01-01T00:00:00Z",
    },
  ],
  total: 1,
};

function makeWrapper(initialActiveId: string | null) {
  let captured: StoreApi<AppState> | null = null;
  function Capture() {
    captured = useAppStoreApi();
    return null;
  }
  function Wrapper({ children }: { children: ReactNode }) {
    return (
      <StateProvider initialState={{ workspaces: { items: [], activeId: initialActiveId } }}>
        <Capture />
        {children}
      </StateProvider>
    );
  }
  return { Wrapper, getStore: () => captured };
}

// CR-001 (round 4 review): `useOfficeRouteBootstrap` used to hydrate
// `workspaces.activeId` directly, bypassing `setActiveWorkspace` and leaving
// `activeIdRevision` unbumped. Consumers keyed on that revision (the
// Failed-inbox cache) could then render a stale, pre-switch snapshot after
// navigating through Office instead of a real `setActiveWorkspace` call.
describe("useOfficeRouteBootstrap", () => {
  it("routes the resolved active workspace through setActiveWorkspace, bumping activeIdRevision", async () => {
    getOnboardingStateMock.mockResolvedValue({ completed: true });
    listWorkspacesMock.mockResolvedValue(OFFICE_WORKSPACES_RESPONSE);
    fetchUserSettingsMock.mockResolvedValue(null);

    const { Wrapper, getStore } = makeWrapper("ws-kanban-1");
    renderHook(() => useOfficeRouteBootstrap(true, null), { wrapper: Wrapper });

    await waitFor(() => {
      expect(getStore()?.getState().workspaces.activeId).toBe(OFFICE_WORKSPACE_ID);
    });
    expect(getStore()?.getState().workspaces.activeIdRevision).toBe(1);
  });

  it("does not bump activeIdRevision when the resolved workspace matches the one already active", async () => {
    getOnboardingStateMock.mockResolvedValue({ completed: true });
    listWorkspacesMock.mockResolvedValue(OFFICE_WORKSPACES_RESPONSE);
    fetchUserSettingsMock.mockResolvedValue(null);

    const { Wrapper, getStore } = makeWrapper(OFFICE_WORKSPACE_ID);
    renderHook(() => useOfficeRouteBootstrap(true, null), { wrapper: Wrapper });

    await waitFor(() => {
      expect(listWorkspacesMock).toHaveBeenCalled();
    });
    expect(getStore()?.getState().workspaces.activeId).toBe(OFFICE_WORKSPACE_ID);
    expect(getStore()?.getState().workspaces.activeIdRevision ?? 0).toBe(0);
  });
});
