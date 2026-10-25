import { afterEach, describe, expect, it, vi } from "vitest";
import { pluginRegistry } from "@/lib/plugins/registry";
import type { PluginIcon, PluginTaskMenuContext } from "@/lib/plugins/types";
import { buildPrimaryPluginEntries } from "./task-menu-actions";

// The registration boundary: what the menu builder does with a registration whose
// own `id` is missing or has no usable string form. The registry guards its own
// read of these objects (see registry.test.ts); these cases cover the builder's
// half, where a value of the wrong shape must drop the entry rather than key it.
const PLUGIN_ID = "kandev-plugin-boundary";

const CONTEXT: PluginTaskMenuContext = {
  workspaceId: "ws-1",
  taskId: "task-1",
  taskTitle: "Fix the bug",
  workflowStepId: "step-1",
  presentation: "desktop",
};

function registerAction(
  overrides: {
    id?: unknown;
    label?: string;
    icon?: PluginIcon;
    items?: unknown;
  } = {},
) {
  pluginRegistry.forPlugin(PLUGIN_ID).registerTaskMenuAction({
    id: overrides.id ?? "action",
    label: overrides.label ?? "Action",
    group: "primary",
    run: vi.fn(),
    ...(overrides.icon ? { icon: overrides.icon } : {}),
    ...(overrides.items ? { items: overrides.items } : {}),
  } as never);
}

afterEach(() => pluginRegistry.unregisterPlugin(PLUGIN_ID));

describe("task menu registration boundary", () => {
  it("omits an action whose id is not a usable string", () => {
    const consoleErrorSpy = vi.spyOn(console, "error").mockImplementation(() => {});
    registerAction({ id: {} as never, items: () => [{ id: "ok", label: "Ok", run: vi.fn() }] });

    expect(buildPrimaryPluginEntries({ context: CONTEXT })).toEqual([]);
    expect(consoleErrorSpy).toHaveBeenCalledWith(
      expect.stringContaining("no usable id"),
      undefined,
    );
    consoleErrorSpy.mockRestore();
  });

  it("omits a registration whose id has no string form, without throwing", () => {
    // A Symbol id or a null-prototype object has no `ToString`: the log line that
    // reports the omission must not be able to throw, or the report itself takes
    // the render down while dropping the registration.
    const consoleErrorSpy = vi.spyOn(console, "error").mockImplementation(() => {});
    registerAction({ id: Symbol("add-tag") as never, label: "Symbol id" });
    registerAction({ id: Object.create(null) as never, label: "Null-prototype id" });

    expect(() => buildPrimaryPluginEntries({ context: CONTEXT })).not.toThrow();
    expect(buildPrimaryPluginEntries({ context: CONTEXT })).toEqual([]);
    expect(consoleErrorSpy).toHaveBeenCalled();
    consoleErrorSpy.mockRestore();
  });
});
