import { cleanup, render } from "@testing-library/react";
import { afterEach, describe, expect, it } from "vitest";
import { TaskStateActions } from "./task-state-actions";

afterEach(cleanup);

describe("TaskStateActions — open-task header status icon", () => {
  it("shows the background spinner (IconLoader) for a background-running task, not the done check", () => {
    // §spec:task-level-indicator: the open-task header status icon reflects the
    // task-level aggregate — background-running (IconLoader), never a done check
    // for a task still doing background work, even when the coarse state is done.
    const { container } = render(
      <TaskStateActions state="COMPLETED" foregroundActivity="background" />,
    );
    expect(container.querySelector(".tabler-icon-loader")).not.toBeNull();
    expect(container.querySelector(".tabler-icon-check")).toBeNull();
    // Distinct by SHAPE from the generating spinner (IconLoader2), not hue alone.
    expect(container.querySelector(".tabler-icon-loader-2")).toBeNull();
  });

  it("shows the generating spinner (IconLoader2) when any session is generating", () => {
    const { container } = render(
      <TaskStateActions state="COMPLETED" foregroundActivity="generating" />,
    );
    expect(container.querySelector(".tabler-icon-loader-2")).not.toBeNull();
    expect(container.querySelector(".tabler-icon-check")).toBeNull();
  });

  it("falls through to the coarse done check when no session is active", () => {
    const { container } = render(<TaskStateActions state="COMPLETED" foregroundActivity={null} />);
    expect(container.querySelector(".tabler-icon-check")).not.toBeNull();
    expect(container.querySelector(".tabler-icon-loader-2")).toBeNull();
  });
});
