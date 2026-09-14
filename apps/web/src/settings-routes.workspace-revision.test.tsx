import type { ReactNode } from "react";
import { render, waitFor } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";
import type { StoreApi } from "zustand";
import { StateProvider, useAppStoreApi } from "@/components/state-provider";
import type { AppState } from "@/lib/state/store";

vi.mock("@/lib/config", () => ({
  getBackendConfig: () => ({ apiBaseUrl: "http://localhost:8443" }),
}));

const listWorkspacesMock = vi.fn();
const listExecutorsMock = vi.fn();
const listAgentsMock = vi.fn();
const listAgentDiscoveryMock = vi.fn();
const listAvailableAgentsMock = vi.fn();
const fetchUserSettingsMock = vi.fn();

vi.mock("@/lib/api/domains/workspace-api", async (importOriginal) => ({
  ...(await importOriginal<object>()),
  listWorkspaces: (...args: unknown[]) => listWorkspacesMock(...args),
}));

vi.mock("@/lib/api/domains/settings-api", async (importOriginal) => ({
  ...(await importOriginal<object>()),
  fetchUserSettings: (...args: unknown[]) => fetchUserSettingsMock(...args),
  listAgentDiscovery: (...args: unknown[]) => listAgentDiscoveryMock(...args),
  listAgents: (...args: unknown[]) => listAgentsMock(...args),
  listAvailableAgents: (...args: unknown[]) => listAvailableAgentsMock(...args),
  listExecutors: (...args: unknown[]) => listExecutorsMock(...args),
}));

import { SettingsRouteBootstrap } from "./settings-routes";

const SETTINGS_WORKSPACE_ID = "ws-settings-1";
const SETTINGS_WORKSPACES_RESPONSE = {
  workspaces: [
    {
      id: SETTINGS_WORKSPACE_ID,
      name: "Settings workspace",
      owner_id: "user-1",
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

function stubApis() {
  listExecutorsMock.mockResolvedValue({ executors: [] });
  listAgentsMock.mockResolvedValue({ agents: [] });
  listAgentDiscoveryMock.mockResolvedValue({ agents: [] });
  listAvailableAgentsMock.mockResolvedValue({ agents: [], tools: [] });
  fetchUserSettingsMock.mockResolvedValue(null);
}

// CR-001 (round 4 review): `SettingsRouteBootstrap` used to hydrate
// `workspaces.activeId` directly, bypassing `setActiveWorkspace` and leaving
// `activeIdRevision` unbumped. Consumers keyed on that revision (the
// Failed-inbox cache) could then render a stale, pre-switch snapshot after
// navigating through Settings instead of a real `setActiveWorkspace` call.
describe("SettingsRouteBootstrap", () => {
  it("routes the resolved active workspace through setActiveWorkspace, bumping activeIdRevision", async () => {
    stubApis();
    listWorkspacesMock.mockResolvedValue(SETTINGS_WORKSPACES_RESPONSE);

    const { Wrapper, getStore } = makeWrapper("ws-kanban-1");
    render(<SettingsRouteBootstrap pathname="/settings" />, { wrapper: Wrapper });

    await waitFor(() => {
      expect(getStore()?.getState().workspaces.activeId).toBe(SETTINGS_WORKSPACE_ID);
    });
    expect(getStore()?.getState().workspaces.activeIdRevision).toBe(1);
  });

  it("does not bump activeIdRevision when the resolved workspace matches the one already active", async () => {
    stubApis();
    listWorkspacesMock.mockResolvedValue(SETTINGS_WORKSPACES_RESPONSE);

    const { Wrapper, getStore } = makeWrapper(SETTINGS_WORKSPACE_ID);
    render(<SettingsRouteBootstrap pathname="/settings" />, { wrapper: Wrapper });

    await waitFor(() => {
      expect(listWorkspacesMock).toHaveBeenCalled();
    });
    expect(getStore()?.getState().workspaces.activeId).toBe(SETTINGS_WORKSPACE_ID);
    expect(getStore()?.getState().workspaces.activeIdRevision ?? 0).toBe(0);
  });
});
