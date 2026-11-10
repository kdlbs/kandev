import { cleanup, renderHook, waitFor } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { AutomationPluginToolGroup } from "@/lib/types/automation";
import { useAutomationPluginTools } from "./use-automation-plugin-tools";

const api = vi.hoisted(() => ({ list: vi.fn() }));
vi.mock("@/lib/api/domains/automation-api", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@/lib/api/domains/automation-api")>()),
  getAutomationPluginToolOptions: api.list,
}));

afterEach(() => {
  cleanup();
  api.list.mockReset();
});

const WORKSPACE_ID = "workspace-1";

const groups: AutomationPluginToolGroup[] = [
  {
    plugin_id: "kandev-security",
    display_name: "Kandev Security",
    version: "1.0.0",
    installation_id: "installation-1",
    policy_version: "policy-1",
    tools: [],
  },
];

describe("useAutomationPluginTools", () => {
  it("loads options only for private automations", async () => {
    api.list.mockResolvedValue(groups);
    const { result, rerender } = renderHook(
      ({ enabled }: { enabled: boolean }) => useAutomationPluginTools(WORKSPACE_ID, enabled),
      { initialProps: { enabled: false } },
    );
    expect(api.list).not.toHaveBeenCalled();
    expect(result.current.loading).toBe(false);

    rerender({ enabled: true });
    await waitFor(() => expect(result.current.groups).toEqual(groups));
    expect(api.list).toHaveBeenCalledWith(WORKSPACE_ID);
    expect(result.current.loading).toBe(false);
  });

  it("keeps a failed load visible without clearing prior groups", async () => {
    api.list.mockResolvedValueOnce(groups);
    const { result } = renderHook(() => useAutomationPluginTools(WORKSPACE_ID, true));
    await waitFor(() => expect(result.current.groups).toEqual(groups));

    api.list.mockRejectedValueOnce(new Error("offline"));
    result.current.reload();
    await waitFor(() => expect(result.current.error).toBe(true));
    expect(result.current.groups).toEqual(groups);
  });

  it("clears groups until the new workspace catalog resolves", async () => {
    api.list.mockResolvedValueOnce(groups).mockImplementationOnce(() => new Promise(() => {}));
    const { result, rerender } = renderHook(
      ({ workspaceId }: { workspaceId: string }) => useAutomationPluginTools(workspaceId, true),
      { initialProps: { workspaceId: "workspace-1" } },
    );
    await waitFor(() => expect(result.current.groups).toEqual(groups));

    rerender({ workspaceId: "workspace-2" });
    await waitFor(() => expect(result.current.loading).toBe(true));
    expect(result.current.groups).toEqual([]);
  });

  it("refreshes the catalog when the window regains focus", async () => {
    api.list.mockResolvedValue(groups);
    renderHook(() => useAutomationPluginTools(WORKSPACE_ID, true));
    await waitFor(() => expect(api.list).toHaveBeenCalledTimes(1));
    window.dispatchEvent(new Event("focus"));
    await waitFor(() => expect(api.list).toHaveBeenCalledTimes(2));
  });

  it("refreshes the displayed policy on demand", async () => {
    const updated = groups.map((group) => ({ ...group, policy_version: "policy-2" }));
    api.list.mockResolvedValueOnce(groups).mockResolvedValueOnce(updated);
    const { result } = renderHook(() => useAutomationPluginTools(WORKSPACE_ID, true));
    await waitFor(() => expect(result.current.groups).toEqual(groups));

    result.current.reload();
    await waitFor(() => expect(result.current.groups).toEqual(updated));
  });
});
