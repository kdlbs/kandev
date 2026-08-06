import { afterEach, describe, expect, it, vi } from "vitest";
import { pluginRegistry } from "@/lib/plugins/registry";
import { buildEditMenuEntry } from "./kanban-card-edit-submenu";
import { buildPluginMenuContext, type Task } from "./kanban-card";

const PLUGIN_ID = "plugin-menu-context";

const task: Task = {
  id: "task-1",
  title: "Context task",
  workflowStepId: "step-1",
};

async function invokePluginAction(presentation: "desktop" | "mobile") {
  const visible = vi.fn(() => true);
  const run = vi.fn();
  pluginRegistry.forPlugin(PLUGIN_ID).registerTaskMenuAction({
    id: `action-${presentation}`,
    label: "Inspect",
    group: "edit",
    visible,
    run,
  });

  const context = buildPluginMenuContext(task, "workspace-1", presentation);
  const entry = buildEditMenuEntry({ context });
  if (entry.kind !== "submenu") throw new Error("expected Edit submenu");
  const action = entry.children.find(
    (child) =>
      child.kind === "item" && child.key === `plugin-edit-${PLUGIN_ID}-action-${presentation}`,
  );
  if (action?.kind !== "item") throw new Error("expected plugin action");

  action.onSelect?.();
  await Promise.resolve();

  return { context, visible, run };
}

afterEach(() => {
  pluginRegistry.unregisterPlugin(PLUGIN_ID);
});

describe("kanban plugin menu presentation context", () => {
  it("passes mobile presentation to visible and run callbacks", async () => {
    const { context, visible, run } = await invokePluginAction("mobile");

    expect(context.presentation).toBe("mobile");
    expect(visible).toHaveBeenCalledWith(context);
    expect(run).toHaveBeenCalledWith(context);
  });

  it("passes desktop presentation to visible and run callbacks", async () => {
    const { context, visible, run } = await invokePluginAction("desktop");

    expect(context.presentation).toBe("desktop");
    expect(visible).toHaveBeenCalledWith(context);
    expect(run).toHaveBeenCalledWith(context);
  });
});
