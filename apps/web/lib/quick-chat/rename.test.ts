import { describe, it, expect, vi, beforeEach } from "vitest";

const mockUpdateTask = vi.hoisted(() => vi.fn());

vi.mock("@/lib/api/domains/kanban-api", () => ({
  updateTask: (...args: unknown[]) => mockUpdateTask(...args),
}));

import { migrateStoredQuickChatNames, persistQuickChatRename } from "./rename";
import { getStoredQuickChatNames, setStoredQuickChatName } from "@/lib/local-storage";

beforeEach(() => {
  vi.clearAllMocks();
  window.localStorage.clear();
  mockUpdateTask.mockResolvedValue(undefined);
});

describe("persistQuickChatRename", () => {
  it("saves the name to the backing task title so other devices see it", async () => {
    await expect(persistQuickChatRename("session-1", "task-1", "Renamed")).resolves.toBe(true);

    expect(mockUpdateTask).toHaveBeenCalledWith("task-1", { title: "Renamed" });
  });

  it("clears any legacy local entry once the name is stored server-side", async () => {
    setStoredQuickChatName("session-1", "Old local name");

    await persistQuickChatRename("session-1", "task-1", "Renamed");

    expect(getStoredQuickChatNames()).toEqual({});
  });

  it("keeps the rename locally when the request fails", async () => {
    mockUpdateTask.mockRejectedValueOnce(new Error("offline"));

    await expect(persistQuickChatRename("session-1", "task-1", "Renamed")).rejects.toThrow(
      "offline",
    );
    expect(getStoredQuickChatNames()).toEqual({ "session-1": "Renamed" });
  });

  it("falls back to local storage for a tab with no task yet", async () => {
    await expect(persistQuickChatRename("setup-session", undefined, "Draft")).resolves.toBe(false);

    expect(mockUpdateTask).not.toHaveBeenCalled();
    expect(getStoredQuickChatNames()).toEqual({ "setup-session": "Draft" });
  });
});

describe("migrateStoredQuickChatNames", () => {
  it("uploads a pre-existing local rename so upgrading does not lose it", async () => {
    const sessions = [{ sessionId: "session-1", taskId: "task-1", name: "Agent - Chat 1" }];

    await migrateStoredQuickChatNames(sessions, { "session-1": "My name" });

    expect(mockUpdateTask).toHaveBeenCalledWith("task-1", { title: "My name" });
    expect(getStoredQuickChatNames()).toEqual({});
  });

  it("skips names the server already agrees with", async () => {
    const sessions = [{ sessionId: "session-1", taskId: "task-1", name: "Same" }];

    await migrateStoredQuickChatNames(sessions, { "session-1": "Same" });

    expect(mockUpdateTask).not.toHaveBeenCalled();
  });

  it("skips sessions with no stored rename", async () => {
    const sessions = [{ sessionId: "session-1", taskId: "task-1", name: "Server name" }];

    await migrateStoredQuickChatNames(sessions, {});

    expect(mockUpdateTask).not.toHaveBeenCalled();
  });

  it("retains the local entry when a migration attempt fails, so it retries", async () => {
    mockUpdateTask.mockRejectedValueOnce(new Error("offline"));
    const sessions = [{ sessionId: "session-1", taskId: "task-1", name: "Server name" }];

    await expect(
      migrateStoredQuickChatNames(sessions, { "session-1": "My name" }),
    ).resolves.toBeUndefined();

    expect(getStoredQuickChatNames()).toEqual({ "session-1": "My name" });
  });
});
