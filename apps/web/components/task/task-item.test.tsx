import { afterEach, describe, expect, it } from "vitest";
import { cleanup, render, screen } from "@testing-library/react";
import type { ComponentProps } from "react";
import { StateProvider } from "@/components/state-provider";
import { TaskItem } from "./task-item";

afterEach(() => cleanup());

function renderTaskItem(props: Partial<ComponentProps<typeof TaskItem>> = {}) {
  return render(
    <StateProvider>
      <TaskItem title="Needs answer" state="REVIEW" {...props} />
    </StateProvider>,
  );
}

describe("TaskItem status icon", () => {
  it("keeps the review check when the session is idle after a turn", () => {
    renderTaskItem({ sessionState: "WAITING_FOR_INPUT" });

    expect(screen.queryByTestId("task-state-review")).not.toBeNull();
    expect(screen.queryByTestId("task-state-waiting-for-input")).toBeNull();
  });

  it("shows question icon when a clarification is pending", () => {
    renderTaskItem({ sessionState: "WAITING_FOR_INPUT", hasPendingClarification: true });

    expect(screen.queryByTestId("task-state-waiting-for-input")).not.toBeNull();
    expect(screen.queryByTestId("task-state-review")).toBeNull();
  });

  it("keeps the review check for completed review tasks", () => {
    renderTaskItem({ sessionState: "COMPLETED" });

    expect(screen.queryByTestId("task-state-review")).not.toBeNull();
    expect(screen.queryByTestId("task-state-waiting-for-input")).toBeNull();
  });
});
