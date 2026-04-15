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
