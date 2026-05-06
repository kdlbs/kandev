import { afterEach, describe, expect, it, vi } from "vitest";
import { act, cleanup, fireEvent, render, screen } from "@testing-library/react";
import type { ReactNode } from "react";
import { StateProvider } from "@/components/state-provider";
import { TaskOptimisticContextProvider } from "@/hooks/use-optimistic-task-mutation";
import { PriorityPicker } from "./priority-picker";
import type { Task } from "@/app/office/tasks/[id]/types";

vi.mock("@/lib/api/domains/office-extended-api", async () => {
  const actual = await vi.importActual<typeof import("@/lib/api/domains/office-extended-api")>(
    "@/lib/api/domains/office-extended-api",
  );
  return {
    ...actual,
    updateTask: vi.fn().mockResolvedValue({ ok: true }),
  };
});

import { updateTask } from "@/lib/api/domains/office-extended-api";

afterEach(() => {
  cleanup();
  vi.clearAllMocks();
});

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

describe("PriorityPicker", () => {
  it("shows the current priority label on the trigger", () => {
    render(
      <Wrapper>
        <PriorityPicker task={task} />
      </Wrapper>,
    );
    const trigger = screen.getByTestId("priority-picker-trigger");
    expect(trigger.textContent).toContain("Medium");
  });

  it("calls updateTask with the new priority when changed", async () => {
    render(
      <Wrapper>
        <PriorityPicker task={task} />
      </Wrapper>,
    );
    fireEvent.click(screen.getByTestId("priority-picker-trigger"));
    const option = await screen.findByTestId("priority-picker-option-high");
    await act(async () => {
      fireEvent.click(option);
    });
    expect(updateTask).toHaveBeenCalledWith("t-1", { priority: "high" });
  });

  it("offers exactly four options with no 'none' value", async () => {
    render(
      <Wrapper>
        <PriorityPicker task={task} />
      </Wrapper>,
    );
    fireEvent.click(screen.getByTestId("priority-picker-trigger"));
    expect(await screen.findByTestId("priority-picker-option-critical")).toBeTruthy();
    expect(screen.getByTestId("priority-picker-option-high")).toBeTruthy();
    expect(screen.getByTestId("priority-picker-option-medium")).toBeTruthy();
    expect(screen.getByTestId("priority-picker-option-low")).toBeTruthy();
    expect(screen.queryByTestId("priority-picker-option-none")).toBeNull();
  });
});
