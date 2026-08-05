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
  } = {},
) {
  pluginRegistry.forPlugin(PLUGIN_ID).registerTaskMenuAction({
    id: "enhance",
    label: ACTION_LABEL,
    group: "edit",
    run: overrides.run ?? vi.fn(),
    ...(overrides.visible ? { visible: overrides.visible } : {}),
  });
}

afterEach(() => {
  cleanup();
  pluginRegistry.unregisterPlugin(PLUGIN_ID);
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
        `plugin-edit-${PLUGIN_ID}-enhance`,
      ]);
    }
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
