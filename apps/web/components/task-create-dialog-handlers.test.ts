import { beforeEach, describe, expect, it, vi } from "vitest";
import { waitFor } from "@testing-library/react";
import { updateUserSettings } from "@/lib/api/domains/settings-api";
import {
  readQueuedTaskCreateLastUsedState,
  resetTaskCreateLastUsedSync,
  syncTaskCreateLastUsed,
} from "./task-create-dialog-handlers";

const PENDING_LAST_USED_SYNC_KEY = "kandev.taskCreateLastUsed.pendingSync";

vi.mock("@/lib/api/domains/settings-api", () => ({
  updateUserSettings: vi.fn(),
}));

describe("syncTaskCreateLastUsed", () => {
  beforeEach(() => {
    window.localStorage.clear();
    resetTaskCreateLastUsedSync({ clearQueued: true });
    vi.mocked(updateUserSettings).mockReset();
  });

  it("persists failed last-used patches and retries them after an in-memory reset", async () => {
    vi.mocked(updateUserSettings).mockRejectedValueOnce(new Error("network"));

    syncTaskCreateLastUsed({ branch: "feature" });

    await waitFor(() => {
      expect(updateUserSettings).toHaveBeenCalledWith({
        task_create_last_used: { branch: "feature" },
      });
    });
    expect(JSON.parse(window.localStorage.getItem(PENDING_LAST_USED_SYNC_KEY) ?? "null")).toEqual({
      branch: "feature",
    });

    resetTaskCreateLastUsedSync({ clearQueued: true });
    vi.mocked(updateUserSettings).mockResolvedValueOnce({ settings: {} } as Awaited<
      ReturnType<typeof updateUserSettings>
    >);

    syncTaskCreateLastUsed({});

    await waitFor(() => {
      expect(updateUserSettings).toHaveBeenLastCalledWith({
        task_create_last_used: { branch: "feature" },
      });
      expect(window.localStorage.getItem(PENDING_LAST_USED_SYNC_KEY)).toBeNull();
    });
  });

  it("retains prior queued fields after a successful sync clears pending state", async () => {
    vi.mocked(updateUserSettings).mockResolvedValue({ settings: {} } as Awaited<
      ReturnType<typeof updateUserSettings>
    >);

    syncTaskCreateLastUsed({ branch: "feature" });
    await waitFor(() => {
      expect(window.localStorage.getItem(PENDING_LAST_USED_SYNC_KEY)).toBeNull();
    });

    syncTaskCreateLastUsed({ agent_profile_id: "agent-2" });
    await waitFor(() => {
      expect(updateUserSettings).toHaveBeenLastCalledWith({
        task_create_last_used: { agent_profile_id: "agent-2" },
      });
    });

    expect(readQueuedTaskCreateLastUsedState()).toMatchObject({
      branch: "feature",
      agentProfileId: "agent-2",
    });
  });

  it("keeps queued fields when dialog close resets pending sync state", async () => {
    vi.mocked(updateUserSettings).mockResolvedValue({ settings: {} } as Awaited<
      ReturnType<typeof updateUserSettings>
    >);

    syncTaskCreateLastUsed({ branch: "feature" });
    await waitFor(() => {
      expect(updateUserSettings).toHaveBeenCalledWith({
        task_create_last_used: { branch: "feature" },
      });
    });

    resetTaskCreateLastUsedSync();

    expect(readQueuedTaskCreateLastUsedState()).toMatchObject({
      branch: "feature",
    });
    expect(readQueuedTaskCreateLastUsedState().agentProfileId).toBeUndefined();
  });
});
