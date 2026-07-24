import { describe, it, expect, vi } from "vitest";
import { PanelPortalManager } from "./panel-portal-manager";

function mockApi() {
  return { title: "", setTitle: vi.fn() } as never;
}

describe("PanelPortalManager.reconcile", () => {
  it("removes portals whose panel is no longer live", () => {
    const mgr = new PanelPortalManager();
    mgr.acquire("panel-a", "chat", {}, mockApi());
    mgr.acquire("panel-b", "terminal", {}, mockApi());

    mgr.reconcile(new Set(["panel-a"]));

    expect(mgr.has("panel-a")).toBe(true);
    expect(mgr.has("panel-b")).toBe(false);
  });

  it("no-ops when all portals are live", () => {
    const mgr = new PanelPortalManager();
    const listener = vi.fn();
    mgr.acquire("panel-a", "chat", {}, mockApi());
    mgr.subscribe(listener);
    listener.mockClear();

    mgr.reconcile(new Set(["panel-a"]));

    expect(mgr.has("panel-a")).toBe(true);
    expect(listener).not.toHaveBeenCalled();
  });

  it("notifies listeners when portals are removed", () => {
    const mgr = new PanelPortalManager();
    const listener = vi.fn();
    mgr.acquire("panel-a", "chat", {}, mockApi());
    mgr.acquire("panel-b", "terminal", {}, mockApi());
    mgr.subscribe(listener);
    listener.mockClear();

    mgr.reconcile(new Set(["panel-a"]));

    expect(listener).toHaveBeenCalledOnce();
  });
});

describe("PanelPortalManager.acquire", () => {
  it("notifies listeners when a remount replaces an existing entry's api", () => {
    const mgr = new PanelPortalManager();
    const listener = vi.fn();
    const firstApi = mockApi();
    mgr.acquire("panel-a", "chat", {}, firstApi);
    mgr.subscribe(listener);
    listener.mockClear();

    const secondApi = mockApi();
    mgr.acquire("panel-a", "chat", {}, secondApi);

    expect(listener).toHaveBeenCalledOnce();
    expect(mgr.get("panel-a")?.api).toBe(secondApi);
  });

  it("does not notify when a redundant acquire reuses the same api instance", () => {
    const mgr = new PanelPortalManager();
    const listener = vi.fn();
    const api = mockApi();
    mgr.acquire("panel-a", "chat", {}, api);
    mgr.subscribe(listener);
    listener.mockClear();

    mgr.acquire("panel-a", "chat", { updated: true }, api);

    expect(listener).not.toHaveBeenCalled();
    expect(mgr.get("panel-a")?.params).toEqual({ updated: true });
  });
});
