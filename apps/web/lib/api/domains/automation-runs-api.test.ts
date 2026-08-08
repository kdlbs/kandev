import { describe, expect, it, vi, beforeEach } from "vitest";

const mockRequest = vi.fn();
vi.mock("@/lib/ws/connection", () => ({
  getWebSocketClient: () => ({ request: mockRequest }),
}));

import { listWorkspaceAutomationRuns } from "./automation-api";

const WORKSPACE = "ws-1";
const ACTION = "automation.runs.list_workspace";

beforeEach(() => {
  mockRequest.mockReset();
});

describe("listWorkspaceAutomationRuns", () => {
  it("asks for the workspace's runs on the workspace action", () => {
    mockRequest.mockResolvedValue({ runs: [] });

    void listWorkspaceAutomationRuns(WORKSPACE);

    expect(mockRequest).toHaveBeenCalledWith(ACTION, { workspace_id: WORKSPACE });
  });

  it("passes a limit through only when one is given", () => {
    mockRequest.mockResolvedValue({ runs: [] });

    void listWorkspaceAutomationRuns(WORKSPACE, 50);

    expect(mockRequest).toHaveBeenCalledWith(ACTION, { workspace_id: WORKSPACE, limit: 50 });
  });

  it("unwraps the runs envelope", async () => {
    mockRequest.mockResolvedValue({ runs: [{ id: "r1" }, { id: "r2" }] });

    const runs = await listWorkspaceAutomationRuns(WORKSPACE);

    expect(runs.map((r) => r.id)).toEqual(["r1", "r2"]);
  });

  it("returns an empty list when the workspace has never fired anything", async () => {
    // The envelope may arrive without a list at all; a workspace with no runs
    // should render its empty state rather than throw on `.map`.
    mockRequest.mockResolvedValue({});

    await expect(listWorkspaceAutomationRuns(WORKSPACE)).resolves.toEqual([]);
  });

  it("survives a null response body", async () => {
    mockRequest.mockResolvedValue(null);

    await expect(listWorkspaceAutomationRuns(WORKSPACE)).resolves.toEqual([]);
  });
});
