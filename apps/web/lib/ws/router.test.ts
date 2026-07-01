import { describe, expect, it } from "vitest";
import type { StoreApi } from "zustand";
import type { AppState } from "@/lib/state/store";
import { registerWsHandlers } from "./router";

describe("registerWsHandlers", () => {
  it("does not register legacy handlers for Query-owned todo and prompt usage events", () => {
    const handlers = registerWsHandlers({} as StoreApi<AppState>);

    expect(handlers).not.toHaveProperty("workflow.created");
    expect(handlers).not.toHaveProperty("workflow.updated");
    expect(handlers).not.toHaveProperty("workflow.deleted");
    expect(handlers).not.toHaveProperty("workflow.step.created");
    expect(handlers).not.toHaveProperty("workflow.step.updated");
    expect(handlers).not.toHaveProperty("workflow.step.deleted");
    expect(handlers).not.toHaveProperty("workspace.created");
    expect(handlers).not.toHaveProperty("workspace.updated");
    expect(handlers).not.toHaveProperty("workspace.deleted");
    expect(handlers).not.toHaveProperty("terminal.output");
    expect(handlers).not.toHaveProperty("session.agent_capabilities");
    expect(handlers).not.toHaveProperty("session.poll_mode_changed");
    expect(handlers).not.toHaveProperty("session.mode_changed");
    expect(handlers).not.toHaveProperty("session.todos_updated");
    expect(handlers).not.toHaveProperty("session.prompt_usage");
    expect(handlers).not.toHaveProperty("session.available_commands");
    expect(handlers).not.toHaveProperty("kanban.update");
    expect(handlers).not.toHaveProperty("task.created");
    expect(handlers).not.toHaveProperty("task.state_changed");

    expect(handlers).toHaveProperty("task.updated");
    expect(handlers).toHaveProperty("task.deleted");
    expect(handlers).toHaveProperty("session.shell.output");
    expect(handlers).toHaveProperty("session.process.output");
    expect(handlers).toHaveProperty("session.process.status");
  });
});
