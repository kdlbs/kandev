import { describe, expect, it } from "vitest";
import * as model from "./builtin-mcp-view-model";
const application = {
  session_id: "session",
  task_id: "task",
  workspace_id: "workspace",
  target_revision: 3,
  request_id: "saved-request",
  row_version: 4,
};
const view = {
  session_id: "session",
  task_id: "task",
  workspace_id: "workspace",
  execution_id: "execution",
  can_retry_application: true,
};
const manifest = {
  version: 1,
  catalog_version: 1,
  surface: "kanban-task",
  workspace_id: "workspace",
  task_id: "task",
  task_revision: 3,
  workspace_revision: 0,
  packages: ["core"],
  origins: { core: "required" },
};
const applied = {
  ...view,
  desired: manifest,
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
    catalog: { kandev: ["step_complete_kandev"] },
    accepted_at: "2026-11-16T19:00:00Z",
  },
  application: { ...application, stage: "applied" },
};
type Status = { state: string; retry: boolean };
const status = (value: unknown): Status =>
  (
    model as unknown as { builtinApplicationStatus: (view: unknown) => Status }
  ).builtinApplicationStatus(value);
describe("saved MCP application status", () => {
  it("separates saved, applying, applied, and failed evidence", () => {
    expect(model).toHaveProperty("builtinApplicationStatus");
    expect(status({ ...view, application: { ...application, stage: "pending" } }).state).toBe(
      "pending",
    );
    for (const stage of ["preparing", "retiring", "starting", "verifying", "accepting"]) {
      expect(status({ ...view, application: { ...application, stage } }).state).toBe("applying");
    }
    expect(status(applied).state).toBe("applied");
    expect(status({ ...view, application: { ...application, stage: "failed" } })).toMatchObject({
      state: "failed",
      retry: true,
    });
  });
  it("does not offer Retry for another owner or without write permission", () => {
    expect(model).toHaveProperty("builtinApplicationStatus");
    expect(
      status({
        ...view,
        can_retry_application: false,
        application: { ...application, stage: "failed" },
      }).retry,
    ).toBe(false);
    expect(
      status({ ...view, application: { ...application, session_id: "foreign", stage: "failed" } }),
    ).toMatchObject({ state: "unknown", retry: false });
  });
});

it("requires current complete catalog evidence before reporting applied", () => {
  expect(status({ ...applied, provider_binding: undefined }).state).toBe("pending");
  expect(
    status({ ...applied, provider_binding: { ...applied.provider_binding, catalog: {} } }).state,
  ).toBe("pending");
  expect(
    status({
      ...applied,
      provider_binding: {
        ...applied.provider_binding,
        scope: { ...applied.provider_binding.scope, execution_id: "old" },
      },
    }).state,
  ).toBe("pending");
});

it("settles an already accepted selection without inventing new catalog evidence", () => {
  const unchanged = {
    ...applied,
    application: { ...application, stage: "superseded", reason: "already_current" },
  };
  expect(status(unchanged).state).toBe("applied");
  expect(status({ ...unchanged, provider_available: false }).state).toBe("pending");
  expect(
    status({ ...unchanged, desired: { ...manifest, packages: ["core", "diagnostics"] } }).state,
  ).toBe("pending");
});

it("does not treat an excluded legacy built-in server as a complete applied catalog", () => {
  expect(
    status({
      ...applied,
      provider_binding: {
        ...applied.provider_binding,
        catalog: {
          ...applied.provider_binding.catalog,
          "kandev.configuration": ["stale_tool_kandev"],
        },
      },
    }).state,
  ).toBe("pending");
});
