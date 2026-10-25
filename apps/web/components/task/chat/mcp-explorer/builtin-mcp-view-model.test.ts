import { expect, it } from "vitest";
import type { BuiltinMCPManifest, BuiltinMCPSessionView } from "@/lib/types/builtin-mcp";
import { builtinPackageState } from "./builtin-mcp-view-model";
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
