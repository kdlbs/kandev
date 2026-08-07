import { describe, expect, it, vi } from "vitest";
import {
  immutablePluginTaskActionContext,
  pluginTaskActionIsVisible,
  runPluginTaskLinkAction,
} from "./task-session-sidebar-link-actions";

describe("runPluginTaskLinkAction", () => {
  it("closes the containing mobile drawer before invoking the plugin callback", () => {
    const calls: string[] = [];
    const closeSurface = vi.fn(() => calls.push("close"));
    const run = vi.fn(() => {
      calls.push("plugin");
      return Promise.resolve();
    });

    runPluginTaskLinkAction(closeSurface, run);

    expect(calls).toEqual(["close", "plugin"]);
  });
});

describe("plugin task action isolation", () => {
  it("deep-clones and freezes repositories before exposing task state", () => {
    const repository = { id: "repo-1", nested: { branch: "main" } };
    const context = immutablePluginTaskActionContext({
      workspaceId: "workspace-1",
      taskId: "task-1",
      pathname: "/t/task-1",
      presentation: "desktop",
      repositories: [repository],
    });
    const exposed = context.repositories[0] as typeof repository;

    expect(exposed).not.toBe(repository);
    expect(exposed.nested).not.toBe(repository.nested);
    expect(Object.isFrozen(context.repositories)).toBe(true);
    expect(Object.isFrozen(exposed)).toBe(true);
    expect(Object.isFrozen(exposed.nested)).toBe(true);
  });

  it("contains a plugin visibility failure and treats the action as hidden", () => {
    const warn = vi.spyOn(console, "warn").mockImplementation(() => undefined);
    const visible = pluginTaskActionIsVisible(
      {
        pluginId: "plugin-a",
        id: "broken",
        label: "Broken",
        placement: "link",
        visible: () => {
          throw new Error("plugin failure");
        },
        run: async () => undefined,
      },
      {
        workspaceId: "workspace-1",
        taskId: "task-1",
        pathname: "/t/task-1",
        presentation: "desktop",
        repositories: [],
      },
    );

    expect(visible).toBe(false);
    expect(warn).toHaveBeenCalledOnce();
    warn.mockRestore();
  });
});
