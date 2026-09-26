import { renderHook } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";
import { useWorkflowAgentProfileEffect } from "./task-create-dialog-effects";
import type { DialogFormState } from "@/components/task-create-dialog-types";
import type { AgentProfileOption } from "@/lib/state/slices";

const BOARD_WORKFLOW_ID = "wf-board";
const REMEMBERED_WORKFLOW_ID = "wf-remembered";
const REMEMBERED_AGENT_ID = "remembered-agent";

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
    expect(fsBefore.setAgentProfileId).not.toHaveBeenCalled();

    const fsAfter = makeFs({
      agentProfileId: cursor.id,
      selectedWorkflowId: "wf-1",
      executorProfileId: "profile-1",
    });
    rerender({ fs: fsAfter, authLoaded: true });

    await new Promise((resolve) => setTimeout(resolve, 10));
    expect(fsAfter.setAgentProfileId).not.toHaveBeenCalled();
  });
});

describe("useWorkflowAgentProfileEffect - effective workflow", () => {
  it("locks the agent profile from the remembered effective workflow", () => {
    const fs = makeFs({ selectedWorkflowId: BOARD_WORKFLOW_ID });
    const workflows = [{ id: REMEMBERED_WORKFLOW_ID, agent_profile_id: REMEMBERED_AGENT_ID }];

    renderHook(() =>
      useWorkflowAgentProfileEffect(
        fs,
        workflows,
        [makeProfile(REMEMBERED_AGENT_ID)],
        [makeProfile(REMEMBERED_AGENT_ID)],
        { effectiveWorkflowId: REMEMBERED_WORKFLOW_ID },
      ),
    );

    expect(fs.setWorkflowAgentProfileId).toHaveBeenCalledWith(REMEMBERED_AGENT_ID);
    expect(fs.setAgentProfileId).toHaveBeenCalledWith(REMEMBERED_AGENT_ID);
  });

  it("restores the compatible last-used profile for a remembered workflow without an override", () => {
    const fs = makeFs({ selectedWorkflowId: BOARD_WORKFLOW_ID });
    const rememberedAgent = makeProfile(REMEMBERED_AGENT_ID);
    const workflows = [{ id: REMEMBERED_WORKFLOW_ID }];

    renderHook(() =>
      useWorkflowAgentProfileEffect(fs, workflows, [rememberedAgent], [rememberedAgent], {
        effectiveWorkflowId: REMEMBERED_WORKFLOW_ID,
        lastUsedAgentProfileId: rememberedAgent.id,
      }),
    );

    expect(fs.setWorkflowAgentProfileId).toHaveBeenCalledWith("");
    expect(fs.setAgentProfileId).toHaveBeenCalledWith(rememberedAgent.id);
  });

  it("clears a remembered workflow lock when the effective workflow disappears", () => {
    const fs = makeFs();
    const workflows = [{ id: REMEMBERED_WORKFLOW_ID, agent_profile_id: REMEMBERED_AGENT_ID }];
    const rememberedAgent = makeProfile(REMEMBERED_AGENT_ID);
    const { rerender } = renderHook(
      ({ effectiveWorkflowId }) =>
        useWorkflowAgentProfileEffect(fs, workflows, [rememberedAgent], [rememberedAgent], {
          effectiveWorkflowId,
        }),
      { initialProps: { effectiveWorkflowId: REMEMBERED_WORKFLOW_ID as string | null } },
    );

    expect(fs.setWorkflowAgentProfileId).toHaveBeenCalledWith(REMEMBERED_AGENT_ID);
    rerender({ effectiveWorkflowId: null });

    expect(fs.setWorkflowAgentProfileId).toHaveBeenLastCalledWith("");
  });
});
