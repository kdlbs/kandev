import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { ConversationForkFormContext } from "./conversation-fork-types";
import { ConversationForkTaskDestinations } from "./conversation-fork-task-destinations";

const { push } = vi.hoisted(() => ({ push: vi.fn() }));

vi.mock("@/components/task-create-dialog", () => ({
  TaskCreateDialog: (props: {
    open: boolean;
    workspaceId: string | null;
    workflowId: string;
    defaultStepId: string;
    conversationFork: ConversationForkFormContext;
    onSuccess: (task: { id: string }) => void;
  }) =>
    props.open ? (
      <div
        data-testid="task-destination"
        data-workspace-id={props.workspaceId}
        data-workflow-id={props.workflowId}
        data-step-id={props.defaultStepId}
        data-fork-id={props.conversationFork.snapshot.descriptor.id}
        data-request-id={props.conversationFork.creationRequestId}
      >
        <button type="button" onClick={() => props.onSuccess({ id: "task-created" })}>
          Create
        </button>
      </div>
    ) : null,
}));

vi.mock("@/lib/routing/client-router", () => ({
  useRouter: () => ({ push }),
}));

vi.mock("./new-subtask-dialog", () => ({
  NewSubtaskDialog: (props: {
    open: boolean;
    parentTaskId: string;
    conversationFork: ConversationForkFormContext;
  }) =>
    props.open ? (
      <div
        data-testid="child-destination"
        data-parent-id={props.parentTaskId}
        data-fork-id={props.conversationFork.snapshot.descriptor.id}
        data-request-id={props.conversationFork.creationRequestId}
      />
    ) : null,
}));

const fork = {
  snapshot: { descriptor: { id: "snapshot-1" } },
  creationRequestId: "request-1",
  onConsumed: vi.fn(),
} as unknown as ConversationForkFormContext;

const sourceTask = {
  id: "task-source",
  title: "Source task",
  workspaceId: "workspace-source",
  workflowId: "workflow-source",
  workflowStepId: "step-source",
} as never;

afterEach(() => {
  cleanup();
  vi.clearAllMocks();
});

describe("ConversationForkTaskDestinations", () => {
  it("keeps the source Kandev workspace and task workflow defaults for a separate task", () => {
    render(
      <ConversationForkTaskDestinations
        destination="task"
        sourceTask={sourceTask}
        workspaceId="workspace-source"
        steps={[]}
        conversationFork={fork}
        onClose={vi.fn()}
      />,
    );

    const form = screen.getByTestId("task-destination");
    expect(form.getAttribute("data-workspace-id")).toBe("workspace-source");
    expect(form.getAttribute("data-workflow-id")).toBe("workflow-source");
    expect(form.getAttribute("data-step-id")).toBe("step-source");
    expect(form.getAttribute("data-fork-id")).toBe("snapshot-1");
    expect(form.getAttribute("data-request-id")).toBe("request-1");
  });

  it("opens the created fork task and consumes its frozen context", () => {
    const onClose = vi.fn();
    render(
      <ConversationForkTaskDestinations
        destination="task"
        sourceTask={sourceTask}
        workspaceId="workspace-source"
        steps={[]}
        conversationFork={fork}
        onClose={onClose}
      />,
    );

    fireEvent.click(screen.getByRole("button", { name: "Create" }));
    expect(fork.onConsumed).toHaveBeenCalledOnce();
    expect(onClose).toHaveBeenCalledOnce();
    expect(push).toHaveBeenCalledWith("/t/task-created");
  });

  it("keeps the source task as the child parent and forwards the same frozen request", () => {
    render(
      <ConversationForkTaskDestinations
        destination="child_task"
        sourceTask={sourceTask}
        workspaceId="workspace-source"
        steps={[]}
        conversationFork={fork}
        onClose={vi.fn()}
      />,
    );

    const form = screen.getByTestId("child-destination");
    expect(form.getAttribute("data-parent-id")).toBe("task-source");
    expect(form.getAttribute("data-fork-id")).toBe("snapshot-1");
    expect(form.getAttribute("data-request-id")).toBe("request-1");
  });
});
