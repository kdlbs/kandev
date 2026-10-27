import { expect, it, vi } from "vitest";
import { createAppStore } from "@/lib/state/store";
import { registerTaskSessionHandlers } from "./agent-session";
import { subscribeSandboxApprovalChanges } from "./sandbox-approvals";

it("invalidates only the current session in its owning store and releases listeners", () => {
  const store = createAppStore();
  const foreignStore = createAppStore();
  const handlers = registerTaskSessionHandlers(store);
  handlers["session.state_changed"]!({
    id: "state",
    type: "notification",
    action: "session.state_changed",
    payload: {
      task_id: "task",
      session_id: "session",
      new_state: "RUNNING",
      queue_incarnation_id: "owner",
      updated_at: "2026-10-24T00:00:00Z",
    },
  });
  const changed = vi.fn();
  const sibling = vi.fn();
  const foreign = vi.fn();
  const unsubscribe = subscribeSandboxApprovalChanges(store, "session", changed);
  const removeSibling = subscribeSandboxApprovalChanges(store, "sibling", sibling);
  const removeForeign = subscribeSandboxApprovalChanges(foreignStore, "session", foreign);
  const handler = handlers["session.sandbox_approvals_changed"];
  expect(handler).toBeTypeOf("function");
  const emit = (task = "task", owner = "owner") =>
    handler!({
      id: "approval",
      type: "notification",
      action: "session.sandbox_approvals_changed",
      payload: { task_id: task, session_id: "session", queue_incarnation_id: owner },
    });
  emit();
  expect(changed).toHaveBeenCalledOnce();
  expect(sibling).not.toHaveBeenCalled();
  expect(foreign).not.toHaveBeenCalled();
  emit("foreign");
  emit("task", "previous");
  expect(changed).toHaveBeenCalledOnce();
  unsubscribe();
  removeSibling();
  removeForeign();
  emit();
  expect(changed).toHaveBeenCalledOnce();
});
