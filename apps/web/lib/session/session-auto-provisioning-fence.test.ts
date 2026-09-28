import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

const STORAGE_KEY = "kandev.bulk-session-removal-fences";

async function fence() {
  return import("./session-auto-provisioning-fence");
}

describe("session auto-provisioning fence", () => {
  beforeEach(() => {
    localStorage.clear();
    vi.resetModules();
  });

  afterEach(() => vi.restoreAllMocks());

  it("preserves another tab's suppression when persisting a new task", async () => {
    const { suppressTaskSessionAutoProvisioning } = await fence();
    suppressTaskSessionAutoProvisioning("task-a");
    localStorage.setItem(STORAGE_KEY, JSON.stringify(["task-b"]));

    suppressTaskSessionAutoProvisioning("task-c");

    expect(JSON.parse(localStorage.getItem(STORAGE_KEY) ?? "[]").sort()).toEqual([
      "task-a",
      "task-b",
      "task-c",
    ]);
  });

  it("keeps the in-memory fence when local storage rejects persistence", async () => {
    const { isTaskSessionAutoProvisioningSuppressed, suppressTaskSessionAutoProvisioning } =
      await fence();
    vi.spyOn(Storage.prototype, "setItem").mockImplementation(() => {
      throw new Error("storage unavailable");
    });

    expect(() => suppressTaskSessionAutoProvisioning("task-a")).not.toThrow();
    expect(isTaskSessionAutoProvisioningSuppressed("task-a")).toBe(true);
  });
});
