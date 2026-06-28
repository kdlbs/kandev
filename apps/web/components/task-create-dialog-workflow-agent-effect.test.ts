import { renderHook } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";
import { useWorkflowAgentProfileEffect } from "./task-create-dialog-effects";
import type { DialogFormState } from "@/components/task-create-dialog-types";
import type { AgentProfileOption } from "@/lib/state/slices";

type Fake = Pick<
  DialogFormState,
  | "agentProfileId"
  | "workflowAgentProfileId"
  | "selectedWorkflowId"
  | "executorProfileId"
  | "setAgentProfileId"
  | "setWorkflowAgentProfileId"
>;

function makeFs(overrides: Partial<Fake> = {}): DialogFormState {
  return {
    agentProfileId: "",
    workflowAgentProfileId: "",
    selectedWorkflowId: null,
    executorProfileId: "profile-1",
    setAgentProfileId: vi.fn(),
    setWorkflowAgentProfileId: vi.fn(),
    ...overrides,
  } as unknown as DialogFormState;
}

function makeProfile(id: string): AgentProfileOption {
  return {
    id,
    label: `agent - ${id}`,
    agent_id: `agent-${id}`,
    agent_name: "agent",
    cli_passthrough: false,
  };
}

describe("useWorkflowAgentProfileEffect - user selections", () => {
  it("preserves a user-picked agent while workflow last-used restore is deferred", async () => {
    const claude = makeProfile("claude");
    const cursor = makeProfile("cursor");
    const workflows = [{ id: "wf-1" }];
    const fsBefore = makeFs({
      agentProfileId: cursor.id,
      selectedWorkflowId: "wf-1",
      executorProfileId: "profile-1",
    });

    const { rerender } = renderHook(
      ({ fs, authLoaded }) =>
        useWorkflowAgentProfileEffect(fs, workflows, [claude, cursor], [claude, cursor], {
          lastUsedAgentProfileId: claude.id,
          authLoaded,
        }),
      { initialProps: { fs: fsBefore, authLoaded: false } },
    );

    await new Promise((resolve) => setTimeout(resolve, 10));
    expect(fsBefore.setAgentProfileId).not.toHaveBeenCalledWith("");
    expect(fsBefore.setAgentProfileId).not.toHaveBeenCalledWith(claude.id);

    const fsAfter = makeFs({
      agentProfileId: cursor.id,
      selectedWorkflowId: "wf-1",
      executorProfileId: "profile-1",
    });
    rerender({ fs: fsAfter, authLoaded: true });

    await new Promise((resolve) => setTimeout(resolve, 10));
    expect(fsAfter.setAgentProfileId).not.toHaveBeenCalledWith("");
    expect(fsAfter.setAgentProfileId).not.toHaveBeenCalledWith(claude.id);
  });
});
