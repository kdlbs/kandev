import { act, render, screen, fireEvent, cleanup } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { DropdownMenu, DropdownMenuContent, DropdownMenuTrigger } from "@kandev/ui/dropdown-menu";
import { pluginRegistry, usePluginRegistry } from "@/lib/plugins/registry";
import type { PluginTaskMenuContext } from "@/lib/plugins/types";
import { KanbanCardDropdownMenuItems } from "./kanban-card-menu-items";
import { buildEditMenuEntry } from "./kanban-card-edit-submenu";

const PLUGIN_ID = "kandev-plugin-notes";
const ACTION_LABEL = "Enhance with AI";
const EDIT_SUBMENU_TESTID = "kanban-edit-submenu";

const CONTEXT: PluginTaskMenuContext = {
  workspaceId: "ws-1",
  taskId: "task-1",
  taskTitle: "Fix the bug",
  workflowStepId: "step-1",
  presentation: "desktop",
};

function renderEntry(onEdit?: () => void, context: PluginTaskMenuContext = CONTEXT) {
  const entry = buildEditMenuEntry({ onEdit, context });
  render(
    <DropdownMenu defaultOpen>
      <DropdownMenuTrigger>open</DropdownMenuTrigger>
      <DropdownMenuContent>
        <KanbanCardDropdownMenuItems entries={[entry]} />
      </DropdownMenuContent>
    </DropdownMenu>,
  );
  return entry;
}

function registerEnhanceAction(
  overrides: {
    run?: () => Promise<void> | void;
    visible?: (context: PluginTaskMenuContext) => boolean;
    items?: (context: PluginTaskMenuContext) => readonly {
      id: string;
      label: string;
      run: (context: PluginTaskMenuContext) => Promise<void> | void;
    }[];
  } = {},
) {
  pluginRegistry.forPlugin(PLUGIN_ID).registerTaskMenuAction({
    id: "enhance",
    label: ACTION_LABEL,
    group: "edit",
    run: overrides.run ?? vi.fn(),
    ...(overrides.visible ? { visible: overrides.visible } : {}),
    ...(overrides.items ? { items: overrides.items } : {}),
  });
}

afterEach(() => {
  cleanup();
  pluginRegistry.unregisterPlugin(PLUGIN_ID);
});

describe("buildEditMenuEntry — a registration the host cannot render", () => {
  // Regression: the submenu-vs-flat decision was made from the number of visible
  // registrations, so an action with no usable label wrapped the native Edit item
  // in a submenu even though its own entry was dropped -- unlike the primary
  // group, which omits such an action entirely.
  it("keeps the flat Edit item when the only edit-group action is unusable", () => {
    const consoleErrorSpy = vi.spyOn(console, "error").mockImplementation(() => {});
    pluginRegistry.forPlugin(PLUGIN_ID).registerTaskMenuAction({
      id: "broken",
      label: "" as never,
      group: "edit",
      run: vi.fn(),
    });

    const entry = buildEditMenuEntry({ onEdit: vi.fn(), context: CONTEXT });

    expect(entry.kind).toBe("item");
    expect(entry.kind === "item" ? entry.key : "").toBe("edit");
    expect(consoleErrorSpy).toHaveBeenCalled();
    consoleErrorSpy.mockRestore();
  });
});

describe("buildEditMenuEntry — AC10 (no plugin actions)", () => {
  it("renders the flat Edit item exactly as before", () => {
    const entry = renderEntry(vi.fn());
    expect(entry.kind).toBe("item");
    expect(screen.getByRole("menuitem", { name: "Edit" })).not.toBeNull();
    expect(screen.queryByText("Edit task")).toBeNull();
  });

  it("clicking the flat item calls onEdit", () => {
    const onEdit = vi.fn();
    renderEntry(onEdit);
    fireEvent.click(screen.getByRole("menuitem", { name: "Edit" }));
    expect(onEdit).toHaveBeenCalledTimes(1);
  });
});

describe("buildEditMenuEntry — AC9 (plugin action registered)", () => {
  it("becomes a submenu with 'Edit task' plus the plugin's item", () => {
    registerEnhanceAction();

    const entry = renderEntry(vi.fn());
    expect(entry.kind).toBe("submenu");
    if (entry.kind === "submenu") {
      expect(entry.children.map((c) => c.key)).toEqual([
        "edit-task",
        `plugin-edit-${PLUGIN_ID}:enhance`,
      ]);
    }
  });

  it("renders an edit action's children inside the Edit submenu", () => {
    registerEnhanceAction({
      items: () => [{ id: "quick", label: "Quick enhance", run: vi.fn() }],
    });

    const entry = buildEditMenuEntry({ onEdit: vi.fn(), context: CONTEXT });
    expect(entry.kind).toBe("submenu");
    if (entry.kind !== "submenu") return;

    const pluginChild = entry.children.find(
      (child) => child.key === `plugin-edit-${PLUGIN_ID}:enhance`,
    );
    expect(pluginChild?.kind).toBe("submenu");
    if (pluginChild?.kind !== "submenu") return;
    expect(pluginChild.children).toHaveLength(1);
    const child = pluginChild.children[0];
    expect(child?.kind).toBe("item");
    if (child?.kind !== "item") return;
    expect(child.label).toBe("Quick enhance");
  });
});

describe("buildEditMenuEntry — native GitHub PR unlink choices", () => {
  it("keeps native unlink choices between Edit task and plugin edit actions", () => {
    registerEnhanceAction();
    const unlink = {
      kind: "item" as const,
      key: "unlink-pr-api",
      label: "Remove acme/api #42 from task",
      onSelect: vi.fn(),
    };

    const entry = buildEditMenuEntry({
      onEdit: vi.fn(),
      context: CONTEXT,
      nativeUnlinkEntries: [unlink],
    });

    expect(entry.kind).toBe("submenu");
    if (entry.kind !== "submenu") return;
    expect(entry.children.map((child) => child.key)).toEqual([
      "edit-task",
      "unlink-pr-api",
      `plugin-edit-${PLUGIN_ID}:enhance`,
    ]);
    const unlinkEntry = entry.children[1];
    expect(unlinkEntry?.kind).toBe("item");
    if (unlinkEntry?.kind === "item") {
      expect(unlinkEntry.label).toBe("Remove acme/api #42 from task");
    }
  });

  it("shows a disabled loading entry while exact task PR records are resolving", () => {
    const entry = buildEditMenuEntry({
      onEdit: vi.fn(),
      context: CONTEXT,
      loadingUnlinkLabel: "Loading pull requests...",
    });

    expect(entry.kind).toBe("submenu");
    if (entry.kind !== "submenu") return;
    expect(entry.children.map((child) => child.key)).toEqual([
      "edit-task",
      "loading-pull-requests",
    ]);
    expect(entry.children[1]).toMatchObject({
      kind: "item",
      label: "Loading pull requests...",
      disabled: true,
    });
  });
});

describe("buildEditMenuEntry — AC11 (run invoked with context, rejection caught, menu still closes)", () => {
  it("invokes run(context) on select", async () => {
    const run = vi.fn().mockResolvedValue(undefined);
    registerEnhanceAction({ run });

    renderEntry(vi.fn());
    fireEvent.click(screen.getByTestId(EDIT_SUBMENU_TESTID));
    const pluginItem = await screen.findByRole("menuitem", { name: ACTION_LABEL });
    fireEvent.click(pluginItem);

    // run() is now called from inside a .then(), i.e. a microtask away —
    // needed so a *synchronous* throw inside run() is also caught (see the
    // "run() exception isolation" describe block below).
    await Promise.resolve();
    expect(run).toHaveBeenCalledWith(CONTEXT);
  });

  it("catches a rejecting run without throwing", async () => {
    const run = vi.fn().mockRejectedValue(new Error("boom"));
    registerEnhanceAction({ run });
    const originalConsoleError = console.error;
    console.error = () => {};

    renderEntry(vi.fn());
    fireEvent.click(screen.getByTestId(EDIT_SUBMENU_TESTID));
    const pluginItem = await screen.findByRole("menuitem", { name: ACTION_LABEL });
    expect(() => fireEvent.click(pluginItem)).not.toThrow();

    await Promise.resolve();
    await Promise.resolve();
    console.error = originalConsoleError;
  });
});

// Regression test for review feedback: buildEditMenuEntry itself reads
// pluginRegistry live, but that's only useful if something in the render
// tree actually re-renders when the registry changes. A card that never
// calls usePluginRegistry() (the bug, before useKanbanCardMenus was fixed
// to call it) would keep showing whatever buildEditMenuEntry returned at
// its *own* first render forever, since nothing else it depends on changes
// when a plugin loads or is disabled later. This mirrors the real fix's
// shape — usePluginRegistry() + buildEditMenuEntry on every render — without
// pulling in KanbanCard's full dependency tree.
function ReactiveEditMenu({ onEdit }: { onEdit: () => void }) {
  usePluginRegistry();
  const entry = buildEditMenuEntry({ onEdit, context: CONTEXT });
  return (
    <DropdownMenu defaultOpen>
      <DropdownMenuTrigger>open</DropdownMenuTrigger>
      <DropdownMenuContent>
        <KanbanCardDropdownMenuItems entries={[entry]} />
      </DropdownMenuContent>
    </DropdownMenu>
  );
}

describe("buildEditMenuEntry reactivity — mounted-card register/unregister", () => {
  it("a card already mounted before the plugin loads gains the action once it registers, with no remount", async () => {
    render(<ReactiveEditMenu onEdit={vi.fn()} />);
    expect(screen.queryByTestId(EDIT_SUBMENU_TESTID)).toBeNull();

    act(() => {
      registerEnhanceAction();
    });

    fireEvent.click(screen.getByTestId(EDIT_SUBMENU_TESTID));
    expect(await screen.findByText("Edit task")).not.toBeNull();
    expect(await screen.findByRole("menuitem", { name: ACTION_LABEL })).not.toBeNull();
  });

  it("disabling the plugin drops its action from an already-mounted card, with no remount", () => {
    registerEnhanceAction();
    render(<ReactiveEditMenu onEdit={vi.fn()} />);
    expect(screen.getByTestId(EDIT_SUBMENU_TESTID)).not.toBeNull();

    act(() => {
      pluginRegistry.unregisterPlugin(PLUGIN_ID);
    });

    expect(screen.queryByTestId(EDIT_SUBMENU_TESTID)).toBeNull();
    expect(screen.getByRole("menuitem", { name: "Edit" })).not.toBeNull();
  });
});

describe("buildEditMenuEntry — AC12 (visible filter)", () => {
  it("hides an action whose visible(context) returns false", () => {
    registerEnhanceAction({ visible: () => false });

    const entry = renderEntry(vi.fn());
    expect(entry.kind).toBe("item");
  });

  it("shows an action whose visible(context) returns true", () => {
    registerEnhanceAction({ visible: (context) => context.taskId === CONTEXT.taskId });

    const entry = renderEntry(vi.fn());
    expect(entry.kind).toBe("submenu");
  });

  // Regression test for review feedback: visible() runs synchronously during
  // buildEditMenuEntry, called from render (not an event handler). Before
  // this fix, a throwing visible() propagated straight out of
  // buildEditMenuEntry and would have crashed the whole kanban card's
  // render, not just hidden this one action.
  it("treats a throwing visible() as hidden rather than crashing the card render", () => {
    const originalConsoleError = console.error;
    console.error = () => {};

    registerEnhanceAction({
      visible: () => {
        throw new Error("visible() blew up");
      },
    });

    let entry: ReturnType<typeof buildEditMenuEntry> | undefined;
    expect(() => {
      entry = renderEntry(vi.fn());
    }).not.toThrow();
    expect(entry?.kind).toBe("item");

    console.error = originalConsoleError;
  });
});

describe("buildEditMenuEntry — run() exception isolation", () => {
  // Regression test for review feedback: Promise.resolve(action.run(context))
  // only catches an *asynchronous* rejection — action.run(context) still runs
  // synchronously as part of evaluating that expression, so a throw inside it
  // happens before Promise.resolve(...) ever produces a promise to attach
  // .catch() to. It escapes uncaught rather than being logged. Asserting on
  // the error log (not fireEvent.click().not.toThrow()) is what actually
  // discriminates the two implementations here: React's synthetic event
  // dispatch already isolates a handler's synchronous throw from the test's
  // own call stack in jsdom, so "click didn't throw" passes either way.
  it("logs and does not silently drop a synchronously-throwing run()", async () => {
    const consoleErrorSpy = vi.spyOn(console, "error").mockImplementation(() => {});
    const run = vi.fn(() => {
      throw new Error("run() blew up synchronously");
    });
    registerEnhanceAction({ run });

    renderEntry(vi.fn());
    fireEvent.click(screen.getByTestId(EDIT_SUBMENU_TESTID));
    const pluginItem = await screen.findByRole("menuitem", { name: ACTION_LABEL });
    fireEvent.click(pluginItem);

    await Promise.resolve();
    await Promise.resolve();
    expect(run).toHaveBeenCalledWith(CONTEXT);
    expect(consoleErrorSpy).toHaveBeenCalledWith(
      expect.stringContaining(`${PLUGIN_ID}:enhance`),
      expect.any(Error),
    );
    consoleErrorSpy.mockRestore();
  });
});
