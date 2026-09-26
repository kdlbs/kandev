import { describe, expect, it, vi } from "vitest";
import {
  immutablePluginTaskActionContext,
  pluginTaskRepositories,
  pluginTaskActionIsVisible,
  runPluginTaskLinkAction,
} from "./task-session-sidebar-link-actions";
import type { Repository } from "@/lib/types/http";

const WORKSPACE_ID = "workspace-1";

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
  it("exposes only linked persisted repository fields in task order", () => {
    const repositories = [
      {
        id: "repo-a",
        workspace_id: WORKSPACE_ID,
        name: "api",
        provider: "bitbucket",
        source_type: "provider",
        provider_repo_id: "workspace/api",
        local_path: "/private/api",
      },
      {
        id: "repo-b",
        workspace_id: WORKSPACE_ID,
        name: "web",
        provider: "github",
        source_type: "provider",
        provider_repo_id: "workspace/web",
        local_path: "/private/web",
      },
    ] as unknown as Repository[];

    expect(
      pluginTaskRepositories(repositories, [
        { repository_id: "repo-b", position: 1 },
        { repository_id: "missing", position: 2 },
        { repository_id: "repo-a", position: 0 },
      ]),
    ).toEqual([
      expect.objectContaining({ id: "repo-a", provider: "bitbucket" }),
      expect.objectContaining({ id: "repo-b", provider: "github" }),
    ]);
    expect(
      pluginTaskRepositories(repositories, [{ repository_id: "repo-a" }])[0],
    ).not.toHaveProperty("local_path");
  });

  it("deep-clones and freezes repositories before exposing task state", () => {
    const repository = {
      id: "repo-1",
      workspace_id: WORKSPACE_ID,
      name: "api",
      provider: "bitbucket",
      nested: { branch: "main" },
    };
    const context = immutablePluginTaskActionContext({
      workspaceId: WORKSPACE_ID,
      taskId: "task-1",
      pathname: "/t/task-1",
      presentation: "desktop",
      repositories: [repository],
    });
    const exposed = context.repositories[0] as unknown as typeof repository;

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
        workspaceId: WORKSPACE_ID,
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
