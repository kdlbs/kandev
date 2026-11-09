import { expect, it } from "vitest";
import type { BuiltinMCPManifest, BuiltinMCPSessionView } from "@/lib/types/builtin-mcp";
import { builtinPackageState, builtinPendingPackages } from "./builtin-mcp-view-model";
const manifest: BuiltinMCPManifest = {
  version: 1,
  catalog_version: 1,
  workspace_id: "ws",
  task_id: "task",
  surface: "kanban-task",
  workspace_revision: 0,
  task_revision: 0,
  packages: ["core", "walkthroughs"],
  origins: { core: "required", walkthroughs: "workspace" },
};
function view(): BuiltinMCPSessionView {
  return {
    workspace_id: "ws",
    task_id: "task",
    session_id: "session",
    execution_id: "execution",
    binding_state: "accepted",
    walkthrough_availability: "available",
    desired: { ...manifest, packages: ["core"], origins: { core: "required" } },
    bound: {
      manifest,
      session_id: "session",
      execution_id: "execution",
      attachment_attempt_id: "attempt",
      accepted_at: "2026-10-25T12:00:00Z",
    },
  };
}
it("does not turn a future exclusion into a current-process exclusion", () => {
  expect(builtinPackageState(view(), "walkthroughs")).toEqual({
    desired: false,
    bound: true,
    pending: true,
  });
});
it("does not infer acceptance from desired selection", () => {
  const value = view();
  delete value.bound;
  value.desired = manifest;
  expect(builtinPackageState(value, "walkthroughs")).toEqual({
    desired: true,
    bound: undefined,
    pending: false,
  });
});

it("lists a future addition and removal without manufacturing attachment evidence", () => {
  const value = view();
  value.desired = {
    ...manifest,
    packages: ["core", "configuration"],
    origins: { core: "required", configuration: "task" },
  };
  expect(builtinPendingPackages(value)).toEqual([
    { id: "configuration", action: "add" },
    { id: "walkthroughs", action: "remove" },
  ]);
  expect(value.bound?.manifest.packages).toEqual(["core", "walkthroughs"]);
  expect(value.history).toBeUndefined();
});
it.each(["unknown", "legacy"] as const)(
  "keeps saved selections visible with %s acceptance without inferring additions",
  (bindingState) => {
    const value = view();
    delete value.bound;
    value.binding_state = bindingState;
    value.desired = {
      ...manifest,
      packages: ["core", "configuration"],
      origins: { core: "required", configuration: "task" },
    };
    expect(builtinPendingPackages(value)).toEqual([
      { id: "core", action: "selected" },
      { id: "configuration", action: "selected" },
    ]);
    expect(builtinPackageState(value, "configuration").bound).toBeUndefined();
    expect(value.history).toBeUndefined();
  },
);
it("shows owned saved choices without comparing foreign acceptance", () => {
  const foreign = view();
  foreign.bound!.execution_id = "foreign";
  expect(builtinPendingPackages(foreign)).toEqual([{ id: "core", action: "selected" }]);
});
it("does not expose a desired selection owned by another task or workspace", () => {
  for (const owner of ["task_id", "workspace_id"] as const) {
    const value = view();
    value.desired![owner] = "foreign";
    expect(builtinPendingPackages(value)).toEqual([]);
  }
});
it("does not claim package changes for revision-only saves", () => {
  const value = view();
  value.desired = { ...manifest, task_revision: 4 };
  expect(builtinPendingPackages(value)).toEqual([]);
});
it.each(["session", "task", "workspace", "execution"])(
  "rejects acceptance owned by another %s",
  (owner) => {
    const value = view();
    if (owner === "session") value.bound!.session_id = "foreign";
    if (owner === "execution") value.bound!.execution_id = "foreign";
    if (owner === "task") value.bound!.manifest = { ...manifest, task_id: "foreign" };
    if (owner === "workspace") value.bound!.manifest = { ...manifest, workspace_id: "foreign" };
    expect(builtinPackageState(value, "walkthroughs").bound).toBeUndefined();
  },
);

it("marks exact reserved native identities, not unrelated hyphenated names", async () => {
  const { isReservedBuiltinName } = await import("./builtin-mcp-view-model");
  for (const name of ["kandev", "kandev-configuration", " KANDEV-WALKTHROUGHS ", "kandev.legacy"])
    expect(isReservedBuiltinName(name)).toBe(true);
  for (const name of ["kandev-example", "kandev-core", "kandev-rich-output-extra"])
    expect(isReservedBuiltinName(name)).toBe(false);
});
