import { describe, expect, it } from "vitest";
import { defaultSidebarLayout, toApiSidebarLayout } from "./layout-types";
import { hasVisibleNewTask } from "./mobile-layout";

describe("phone primary creation visibility", () => {
  it("uses the primary action for a default layout", () => {
    expect(hasVisibleNewTask(undefined)).toBe(true);
    expect(
      defaultSidebarLayout()
        .nodes.slice(0, 2)
        .map((node) => node.destinationId),
    ).toEqual(["new_task", "home"]);
  });

  it("retains the inline fallback when the user hides New Task", () => {
    const layout = defaultSidebarLayout();
    layout.revision = 1;
    layout.nodes.find((node) => node.destinationId === "new_task")!.visible = false;
    expect(hasVisibleNewTask(toApiSidebarLayout(layout))).toBe(false);
    expect(layout.nodes.find((node) => node.destinationId === "new_task")?.visible).toBe(false);
  });

  it("does not treat a custom shortcut as the built-in action", () => {
    const layout = defaultSidebarLayout();
    layout.revision = 1;
    layout.nodes = [
      {
        id: "shortcuts",
        kind: "shortcuts",
        visible: true,
        shortcuts: [{ id: "new", target: { kind: "host_action", id: "new_task" } }],
      },
    ];
    expect(hasVisibleNewTask(toApiSidebarLayout(layout))).toBe(false);
  });
});
