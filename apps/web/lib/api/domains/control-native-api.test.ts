import { beforeEach, describe, expect, it, vi } from "vitest";
import { fetchJson } from "../client";
import {
  readNativeDashboard,
  readNativeTasks,
  NativeControlContractError,
} from "./control-native-api";
vi.mock("../client", () => ({ fetchJson: vi.fn() }));
beforeEach(() => {
  vi.mocked(fetchJson).mockReset();
});
describe("existing authorized native control routes", () => {
  it("opts into the existing dashboard and rejects the old response shape", async () => {
    vi.mocked(fetchJson).mockResolvedValue({ workspace_id: "w", agents: [] });
    await expect(readNativeDashboard("w")).rejects.toBeInstanceOf(NativeControlContractError);
    expect(fetchJson).toHaveBeenCalledWith(
      "/api/v1/office/workspaces/w/dashboard?view=control",
      undefined,
    );
  });
  it("preserves archive selection and explicit all-match intent", async () => {
    vi.mocked(fetchJson).mockResolvedValue({
      kind: "tasks",
      filter: "completed",
      archive: "all",
      total: 0,
      truncated: false,
      generated_at: "2026-10-19T03:00:00Z",
      tasks: [],
    });
    await readNativeTasks("w", "completed", "all", -1);
    expect(fetchJson).toHaveBeenCalledWith(
      "/api/v1/office/workspaces/w/tasks?view=control&filter=completed&archive=all&limit=-1",
      undefined,
    );
  });
});
