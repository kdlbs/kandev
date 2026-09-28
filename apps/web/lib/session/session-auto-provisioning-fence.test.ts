import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

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
    const firstTab = await fence();
    firstTab.suppressTaskSessionAutoProvisioning("task-a");
    vi.resetModules();
    const secondTab = await fence();
    secondTab.suppressTaskSessionAutoProvisioning("task-b");
    firstTab.suppressTaskSessionAutoProvisioning("task-c");
    vi.resetModules();
    const reloadedTab = await fence();

    expect(reloadedTab.isTaskSessionAutoProvisioningSuppressed("task-a")).toBe(true);
    expect(reloadedTab.isTaskSessionAutoProvisioningSuppressed("task-b")).toBe(true);
    expect(reloadedTab.isTaskSessionAutoProvisioningSuppressed("task-c")).toBe(true);
  });

  it("does not restore a fence cleared by another tab", async () => {
    const firstTab = await fence();
    firstTab.suppressTaskSessionAutoProvisioning("task-a");
    vi.resetModules();
    const secondTab = await fence();
    secondTab.clearTaskSessionAutoProvisioningSuppression("task-a");

    firstTab.suppressTaskSessionAutoProvisioning("task-c");
    expect(firstTab.isTaskSessionAutoProvisioningSuppressed("task-a")).toBe(false);
    vi.resetModules();
    const reloadedTab = await fence();
    expect(reloadedTab.isTaskSessionAutoProvisioningSuppressed("task-a")).toBe(false);
    expect(reloadedTab.isTaskSessionAutoProvisioningSuppressed("task-c")).toBe(true);
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

  it("honors a local clear when browser storage rejects removal", async () => {
    const {
      clearTaskSessionAutoProvisioningSuppression,
      isTaskSessionAutoProvisioningSuppressed,
      suppressTaskSessionAutoProvisioning,
    } = await fence();
    suppressTaskSessionAutoProvisioning("task-a");
    vi.spyOn(Storage.prototype, "removeItem").mockImplementation(() => {
      throw new Error("storage unavailable");
    });

    clearTaskSessionAutoProvisioningSuppression("task-a");

    expect(isTaskSessionAutoProvisioningSuppressed("task-a")).toBe(false);
  });
});
