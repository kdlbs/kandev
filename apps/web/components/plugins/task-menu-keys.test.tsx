import { afterEach, describe, expect, it, vi } from "vitest";
import { pluginRegistry } from "@/lib/plugins/registry";
import type { PluginTaskMenuContext } from "@/lib/plugins/types";
import { buildPrimaryPluginEntries } from "./task-menu-actions";

// Key identity. A plugin entry key is a palette row's React key and cmdk value,
// so it has to be unique for any strings a bundle can author -- including the
// shapes that make a naive escape and a bare separator collide.
const PLUGIN_ID = "kandev-plugin-keys";

const CONTEXT: PluginTaskMenuContext = {
  workspaceId: "ws-1",
  taskId: "task-1",
  taskTitle: "Fix the bug",
  workflowStepId: "step-1",
  presentation: "desktop",
};

afterEach(() => {
  pluginRegistry.unregisterPlugin(PLUGIN_ID);
  pluginRegistry.unregisterPlugin("p");
  pluginRegistry.unregisterPlugin("p-a");
  pluginRegistry.unregisterPlugin("p-q");
  pluginRegistry.unregisterPlugin("p-pick");
  pluginRegistry.unregisterPlugin("p\uABCD");
});

describe("plugin entry keys — escapes and separators", () => {
  it("keeps keys distinct when an id ends where another's escape token begins", () => {
    // The separator cannot be read as the start of an escape: with a `%XX`-style
    // token and a bare `%` separator, plugin `p\uABCD` with action `abcd` and
    // plugin `p` with action `abcd\uABCD` both produced `plugin-primary-p%abcd%abcd`.
    pluginRegistry.forPlugin("p\uABCD").registerTaskMenuAction({
      id: "abcd",
      label: "Shifted",
      group: "primary",
      run: vi.fn(),
    });
    pluginRegistry.forPlugin("p").registerTaskMenuAction({
      id: "abcd\uABCD",
      label: "Trailing",
      group: "primary",
      run: vi.fn(),
    });
    pluginRegistry.forPlugin("p").registerTaskMenuAction({
      id: "abcd",
      label: "Parent",
      group: "primary",
      items: () => [
        { id: "\uABCD", label: "Child unit", run: vi.fn() },
        { id: "%3A", label: "Looks like an escape", run: vi.fn() },
      ],
      run: vi.fn(),
    });

    const entries = buildPrimaryPluginEntries({ context: CONTEXT });
    const keys = entries.flatMap((entry) =>
      entry
        ? [entry.key, ...(entry.kind === "submenu" ? entry.children.map((c) => c.key) : [])]
        : [],
    );

    expect(keys).toHaveLength(5);
    expect(new Set(keys).size).toBe(5);

    for (const pluginId of ["p\uABCD", "p"]) pluginRegistry.unregisterPlugin(pluginId);
  });

  it("keeps keys distinct when an id carries the child delimiter or a dash", () => {
    // A `#` inside an action id must not forge a sibling child's key, and a `-`
    // must not let one (pluginId, actionId) pair spell another's.
    pluginRegistry.forPlugin("p").registerTaskMenuAction({
      id: "quick",
      label: "Quick",
      group: "primary",
      items: () => [{ id: "x", label: "Child", run: vi.fn() }],
      run: vi.fn(),
    });
    pluginRegistry.forPlugin("p").registerTaskMenuAction({
      id: "quick#x",
      label: "Hashed",
      group: "primary",
      run: vi.fn(),
    });
    pluginRegistry.forPlugin("p").registerTaskMenuAction({
      id: "a-b",
      label: "Dashed",
      group: "primary",
      run: vi.fn(),
    });
    pluginRegistry.forPlugin("p-a").registerTaskMenuAction({
      id: "b",
      label: "Shifted",
      group: "primary",
      run: vi.fn(),
    });

    const entries = buildPrimaryPluginEntries({ context: CONTEXT });
    const keys = entries.flatMap((entry) =>
      entry
        ? [entry.key, ...(entry.kind === "submenu" ? entry.children.map((c) => c.key) : [])]
        : [],
    );

    expect(keys).toHaveLength(5);
    expect(new Set(keys).size).toBe(5);
    // A flat key never contains the raw child delimiter, so it cannot equal a child key.
    expect(keys.filter((key) => key.includes("#"))).toHaveLength(1);

    for (const pluginId of ["p", "p-a"]) pluginRegistry.unregisterPlugin(pluginId);
  });
});

describe("plugin entry keys — dash joins", () => {
  it("keeps child keys distinct when two actions' ids dash-join identically", () => {
    // Every key here is a dash-join (before this fix) of plugin-controlled ids,
    // and the palette flattens all plugin entries into one list where a key is
    // both a React key and cmdk's value. Both pairs below collided under a
    // dash-join: a child against a flat action's key, and two actions whose id
    // joins agree (`p` + `q-x` versus `p-q` + `x`).
    pluginRegistry.forPlugin("p").registerTaskMenuAction({
      id: "pick",
      label: "Pick",
      group: "primary",
      items: () => [{ id: "x", label: "A", run: vi.fn() }],
      run: vi.fn(),
    });
    pluginRegistry.forPlugin("p-pick").registerTaskMenuAction({
      id: "pick",
      label: "Pick x",
      group: "primary",
      items: () => [{ id: "pick-x", label: "B", run: vi.fn() }],
      run: vi.fn(),
    });
    pluginRegistry.forPlugin("p").registerTaskMenuAction({
      id: "q-x",
      label: "Q x",
      group: "primary",
      items: () => [{ id: "c", label: "C", run: vi.fn() }],
      run: vi.fn(),
    });
    pluginRegistry.forPlugin("p-q").registerTaskMenuAction({
      id: "x",
      label: "X",
      group: "primary",
      items: () => [{ id: "c", label: "D", run: vi.fn() }],
      run: vi.fn(),
    });

    const entries = buildPrimaryPluginEntries({ context: CONTEXT });
    const keys = entries.map((entry) => entry.key);
    const childKeys = entries.flatMap((entry) =>
      entry && entry.kind === "submenu" ? entry.children.map((child) => child.key) : [],
    );

    expect(keys).toHaveLength(4);
    expect(new Set(keys).size).toBe(4);
    expect(childKeys).toHaveLength(4);
    expect(new Set(childKeys).size).toBe(4);
    // A child key can never equal a flat action key either.
    expect(childKeys.filter((key) => keys.includes(key))).toEqual([]);

    for (const pluginId of ["p", "p-pick", "p-q"]) pluginRegistry.unregisterPlugin(pluginId);
  });
});
