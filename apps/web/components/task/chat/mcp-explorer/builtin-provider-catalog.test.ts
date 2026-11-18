import { expect, it } from "vitest";
import * as model from "./builtin-provider-binding";
import type { BuiltinMCPManifest, BuiltinMCPSessionView } from "@/lib/types/builtin-mcp";
import type { MCPAttachmentServer } from "@/lib/state/slices/session-runtime/types";
const manifest: BuiltinMCPManifest = {
  workspace_revision: 0,
  task_revision: 3,
  version: 1,
  catalog_version: 1,
  surface: "kanban-task",
  workspace_id: "ws",
  task_id: "task",
  packages: ["core", "configuration"],
  origins: { core: "required", configuration: "task" },
};
const view: BuiltinMCPSessionView = {
  binding_state: "accepted",
  walkthrough_availability: "unavailable",
  session_id: "session",
  task_id: "task",
  workspace_id: "ws",
  execution_id: "execution",
  provider_available: true,
  provider_binding: {
    session_id: "session",
    scope: {
      binding_id: "current",
      execution_id: "execution",
      helper_id: "helper",
      process_generation: 2,
      harness_generation: 1,
      profile_digest: "a".repeat(64),
    },
    manifest,
    catalog: {
      kandev: ["step_complete_kandev"],
      "kandev-configuration": ["list_executors_kandev"],
    },
    accepted_at: "2026-11-16T19:00:00Z",
  },
};
const observed: MCPAttachmentServer[] = [
  {
    name: "kandev-diagnostics",
    source: "kandev",
    builtin_package: "diagnostics",
    status: "active",
    tools: [{ name: "old_diagnostic_kandev" }],
  },
  { name: "external", source: "profile", status: "connected" },
];
const project = (v: BuiltinMCPSessionView) =>
  (
    model as unknown as {
      providerAwareServers: (
        servers: MCPAttachmentServer[],
        view: BuiltinMCPSessionView,
      ) => MCPAttachmentServer[];
    }
  ).providerAwareServers(observed, v);
it("shows the accepted provider catalog instead of stale attachment tools", () => {
  expect(model).toHaveProperty("providerAwareServers");
  const servers = project(view);
  expect(servers.map((s) => s.name)).toEqual(["external", "kandev", "kandev-configuration"]);
  expect(servers.find((s) => s.builtin_package === "configuration")).toMatchObject({
    status: "active",
    tools: [{ name: "list_executors_kandev" }],
  });
  expect(servers[0]).toBe(observed[1]);
});
it("does not present historical tools as active while provider effects are closed", () => {
  expect(model).toHaveProperty("providerAwareServers");
  const servers = project({ ...view, provider_available: false });
  expect(servers.find((s) => s.source === "kandev")).toMatchObject({
    status: "unavailable",
    tools: undefined,
    tool_count: undefined,
  });
  expect(observed[0].tools).toHaveLength(1);
});
it("does not replace legacy evidence with a foreign or incomplete snapshot", () => {
  expect(model).toHaveProperty("providerAwareServers");
  expect(
    project({ ...view, provider_binding: { ...view.provider_binding!, session_id: "foreign" } }),
  ).toBe(observed);
});
