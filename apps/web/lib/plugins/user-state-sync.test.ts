import { afterEach, describe, expect, it, vi } from "vitest";
import { pluginRegistry } from "./registry";
import {
  composeWriterId,
  PLUGIN_USER_STATE_UPDATED_ACTION,
  subscribeToUserStateChanges,
} from "./user-state-sync";

function dispatch(payload: unknown) {
  pluginRegistry
    .getWsHandlers(PLUGIN_USER_STATE_UPDATED_ACTION)
    .forEach((handler) => handler(payload));
}

afterEach(() => {
  pluginRegistry.unregisterPlugin("plugin-a");
  pluginRegistry.unregisterPlugin("plugin-b");
});

describe("subscribeToUserStateChanges", () => {
  it("delivers a matching change for the subscribed plugin", () => {
    const handler = vi.fn();
    subscribeToUserStateChanges("plugin-a", "tab-1", {}, handler);

    dispatch({
      pluginId: "plugin-a",
      scope: "task",
      scopeId: "task_1",
      key: "note",
      updatedAt: "2026-01-01T00:00:00Z",
      writerId: "tab-2",
    });

    expect(handler).toHaveBeenCalledWith({
      scope: "task",
      scopeId: "task_1",
      key: "note",
      updatedAt: "2026-01-01T00:00:00Z",
      deleted: undefined,
    });
  });

  it("ignores notifications for a different plugin (AC25 own-plugin filter)", () => {
    const handler = vi.fn();
    subscribeToUserStateChanges("plugin-a", "tab-1", {}, handler);

    dispatch({
      pluginId: "plugin-b",
      scope: "task",
      scopeId: "task_1",
      key: "note",
      writerId: "tab-2",
    });

    expect(handler).not.toHaveBeenCalled();
  });

  it("suppresses its own tab's echo (AC25)", () => {
    const handler = vi.fn();
    subscribeToUserStateChanges("plugin-a", "tab-1", {}, handler);

    dispatch({
      pluginId: "plugin-a",
      scope: "task",
      scopeId: "task_1",
      key: "note",
      writerId: "tab-1",
    });

    expect(handler).not.toHaveBeenCalled();
  });

  it("filters by scope/scopeId/key when the filter narrows them", () => {
    const handler = vi.fn();
    subscribeToUserStateChanges(
      "plugin-a",
      "tab-1",
      { scope: "task", scopeId: "task_1", key: "note" },
      handler,
    );

    dispatch({
      pluginId: "plugin-a",
      scope: "task",
      scopeId: "task_2",
      key: "note",
      writerId: "tab-2",
    });
    expect(handler).not.toHaveBeenCalled();

    dispatch({
      pluginId: "plugin-a",
      scope: "task",
      scopeId: "task_1",
      key: "note",
      writerId: "tab-2",
    });
    expect(handler).toHaveBeenCalledTimes(1);
  });
});

describe("composeWriterId", () => {
  it("keeps the base id when there is no surface discriminator", () => {
    expect(composeWriterId("tab-1", undefined)).toBe("tab-1");
  });

  it("uses the same separator and order for surface-scoped ids", () => {
    expect(composeWriterId("tab-1", "panel-xyz")).toBe("tab-1:panel-xyz");
  });
});

describe("subscribeToUserStateChanges — filter.writerId per-surface scoping", () => {
  // Regression test for the cross-surface suppression bug found in review:
  // a task panel (subscribed, its own writes tagged with its panelId) and a
  // kanban menu action (write-only, using the tab-wide default) share one
  // browser tab. Before filter.writerId existed, both looked like the same
  // "tab-1" writer to any subscriber, so the panel's own subscription
  // treated the action's write as its own echo and silently dropped it.
  it("scopes echo suppression to one surface, so a sibling surface's write in the same tab still reaches it", () => {
    const panelHandler = vi.fn();
    // The task panel subscribes under its own panelId, not the tab default.
    subscribeToUserStateChanges("plugin-a", "tab-1", { writerId: "panel-xyz" }, panelHandler);

    // The kanban action writes under the tab-wide default writerId — it has
    // no ongoing subscription of its own to protect.
    dispatch({
      pluginId: "plugin-a",
      scope: "task",
      scopeId: "task_1",
      key: "note",
      writerId: "tab-1",
    });

    expect(panelHandler).toHaveBeenCalledTimes(1);
  });

  it("still suppresses this surface's own echo, not just the tab default", () => {
    const panelHandler = vi.fn();
    subscribeToUserStateChanges("plugin-a", "tab-1", { writerId: "panel-xyz" }, panelHandler);

    // The actual comparator is "localWriterId:filter.writerId" (see below) —
    // this panel's own writes are stamped with that same combined id.
    dispatch({
      pluginId: "plugin-a",
      scope: "task",
      scopeId: "task_1",
      key: "note",
      writerId: "tab-1:panel-xyz",
    });

    expect(panelHandler).not.toHaveBeenCalled();
  });

  // Regression test for a bug introduced while fixing the above: a surface
  // id like panelId is a *static* string shared by every browser tab that
  // has that panel open. If filter.writerId fully replaced localWriterId
  // (instead of being combined with it), two different tabs — each with
  // their own random per-tab localWriterId but subscribing under the same
  // panelId — would look like the same writer to each other, and a write
  // in tab A would never reach tab B's subscription (breaking AC24/AC25's
  // actual cross-tab sync, which is what this feature is for).
  it("the same writerId in two different tabs (different localWriterId) does not cross-suppress", () => {
    const tabBHandler = vi.fn();
    // Tab B subscribes under the same panelId as tab A would, but a
    // different tab-unique base — simulating two browser tabs with the
    // same panel open.
    subscribeToUserStateChanges("plugin-a", "tab-B-base", { writerId: "panel-xyz" }, tabBHandler);

    // Tab A writes under its own combined id (its own tab-unique base,
    // same panelId discriminator).
    dispatch({
      pluginId: "plugin-a",
      scope: "task",
      scopeId: "task_1",
      key: "note",
      writerId: "tab-A-base:panel-xyz",
    });

    expect(tabBHandler).toHaveBeenCalledTimes(1);
  });
});

describe("subscribeToUserStateChanges — unsubscribe and revocation", () => {
  it("stops delivering after unsubscribe, without disturbing other subscriptions", () => {
    const handlerA = vi.fn();
    const handlerB = vi.fn();
    const unsubscribeA = subscribeToUserStateChanges("plugin-a", "tab-1", {}, handlerA);
    subscribeToUserStateChanges("plugin-a", "tab-1", {}, handlerB);

    unsubscribeA();
    dispatch({
      pluginId: "plugin-a",
      scope: "task",
      scopeId: "task_1",
      key: "note",
      writerId: "tab-2",
    });

    expect(handlerA).not.toHaveBeenCalled();
    expect(handlerB).toHaveBeenCalledTimes(1);
  });

  // AC26: disabling/uninstalling a plugin bulk-revokes every subscription it
  // holds (via the existing unregisterPlugin path), and re-subscribing after
  // a reload still reaches the registry.
  it("bulk-revokes on unregisterPlugin, and a fresh subscribe after reload works again", () => {
    const handler = vi.fn();
    subscribeToUserStateChanges("plugin-a", "tab-1", {}, handler);
    expect(pluginRegistry.getWsHandlers(PLUGIN_USER_STATE_UPDATED_ACTION)).toHaveLength(1);

    pluginRegistry.unregisterPlugin("plugin-a");
    expect(pluginRegistry.getWsHandlers(PLUGIN_USER_STATE_UPDATED_ACTION)).toHaveLength(0);

    const reloadedHandler = vi.fn();
    subscribeToUserStateChanges("plugin-a", "tab-1", {}, reloadedHandler);
    dispatch({
      pluginId: "plugin-a",
      scope: "task",
      scopeId: "task_1",
      key: "note",
      writerId: "tab-2",
    });

    expect(handler).not.toHaveBeenCalled();
    expect(reloadedHandler).toHaveBeenCalledTimes(1);
  });
});
