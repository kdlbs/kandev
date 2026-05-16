import { afterEach, describe, expect, it, vi } from "vitest";
import { act, cleanup, fireEvent, render, screen } from "@testing-library/react";
import type { ReactNode } from "react";
import { StateProvider } from "@/components/state-provider";
import { TaskOptimisticContextProvider } from "@/hooks/use-optimistic-task-mutation";
import { ApiError } from "@/lib/api/client";
import { StatusPicker, formatPendingApproversMessage } from "./status-picker";
import type { Task } from "@/app/office/tasks/[id]/types";

const updateTaskMock = vi.hoisted(() => vi.fn().mockResolvedValue({ ok: true }));
const toastErrorMock = vi.hoisted(() => vi.fn());

vi.mock("@/lib/api/domains/office-extended-api", async () => {
  const actual = await vi.importActual<typeof import("@/lib/api/domains/office-extended-api")>(
    "@/lib/api/domains/office-extended-api",
  );
  return {
    ...actual,
    updateTask: updateTaskMock,
  };
});

vi.mock("sonner", async () => {
  const actual = await vi.importActual<typeof import("sonner")>("sonner");
  return {
    ...actual,
    toast: { ...actual.toast, error: toastErrorMock },
  };
});

import { updateTask } from "@/lib/api/domains/office-extended-api";

afterEach(() => {
  cleanup();
  vi.clearAllMocks();
  updateTaskMock.mockResolvedValue({ ok: true });
});

const TRIGGER_TEST_ID = "status-picker-trigger";

const task: Task = {
  id: "t-1",
  workspaceId: "ws-1",
  identifier: "TASK-1",
  title: "First task",
  status: "todo",
  priority: "medium",
  labels: [],
  blockedBy: [],
  blocking: [],
  children: [],
  reviewers: [],
  approvers: [],
  decisions: [],
  createdBy: "user",
  createdAt: "2026-05-01T00:00:00Z",
  updatedAt: "2026-05-01T00:00:00Z",
};

function Wrapper({ children }: { children: ReactNode }) {
  const ctx = {
    task,
    applyPatch: vi.fn(),
    restore: vi.fn(),
  };
  return (
    <StateProvider>
      <TaskOptimisticContextProvider value={ctx}>{children}</TaskOptimisticContextProvider>
    </StateProvider>
  );
}

describe("StatusPicker", () => {
  it("renders the current status label on the trigger", () => {
    render(
      <Wrapper>
        <StatusPicker task={task} />
      </Wrapper>,
    );
    const trigger = screen.getByTestId(TRIGGER_TEST_ID);
    expect(trigger.getAttribute("aria-haspopup")).toBe("listbox");
    expect(trigger.textContent).toContain("Todo");
  });

  it("calls updateTask with the new status when an option is selected", async () => {
    render(
      <Wrapper>
        <StatusPicker task={task} />
      </Wrapper>,
    );
    fireEvent.click(screen.getByTestId(TRIGGER_TEST_ID));
    const option = await screen.findByTestId("status-picker-option-in_progress");
    await act(async () => {
      fireEvent.click(option);
    });
    expect(updateTask).toHaveBeenCalledWith("t-1", { status: "in_progress" });
  });

  it("does not call the API when the same status is re-selected", async () => {
    render(
      <Wrapper>
        <StatusPicker task={task} />
      </Wrapper>,
    );
    fireEvent.click(screen.getByTestId(TRIGGER_TEST_ID));
    const option = await screen.findByTestId("status-picker-option-todo");
    await act(async () => {
      fireEvent.click(option);
    });
    expect(updateTask).not.toHaveBeenCalled();
  });

  it("toasts the formatted approvers message on a 409 gate response", async () => {
    updateTaskMock.mockRejectedValueOnce(
      new ApiError("approvals pending", 409, {
        error: "approvals pending",
        pending_approvers: [
          { agent_profile_id: "a1", name: "CEO" },
          { agent_profile_id: "a2", name: "Eng Lead" },
        ],
        status: "in_review",
      }),
    );
    render(
      <Wrapper>
        <StatusPicker task={task} />
      </Wrapper>,
    );
    fireEvent.click(screen.getByTestId(TRIGGER_TEST_ID));
    const option = await screen.findByTestId("status-picker-option-done");
    await act(async () => {
      fireEvent.click(option);
    });
    expect(toastErrorMock).toHaveBeenCalledWith(
      "Cannot mark done: awaiting approval from CEO, Eng Lead",
    );
  });
});

describe("formatPendingApproversMessage", () => {
  it("joins names with commas", () => {
    expect(
      formatPendingApproversMessage([
        { agent_profile_id: "a", name: "CEO" },
        { agent_profile_id: "b", name: "Eng Lead" },
      ]),
    ).toBe("Cannot mark done: awaiting approval from CEO, Eng Lead");
  });

  it("falls back to the agent id when name is missing", () => {
    expect(formatPendingApproversMessage([{ agent_profile_id: "agent-x" }])).toBe(
      "Cannot mark done: awaiting approval from agent-x",
    );
  });
});
