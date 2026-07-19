import { cleanup, render } from "@testing-library/react";
import { afterEach, describe, expect, it } from "vitest";
import { TaskStateActions } from "./task-state-actions";

afterEach(cleanup);

const ICON_CHECK = ".tabler-icon-check";
const ICON_LOADER2 = ".tabler-icon-loader-2";

describe("TaskStateActions — open-task header status icon", () => {
  it("shows the background spinner (IconLoader) for a background-running task, not the done check", () => {
    // §spec:task-level-indicator: the open-task header status icon reflects the
    // task-level aggregate — background-running (IconLoader), never a done check
    // for a task still doing background work, even when the coarse state is done.
    const { container } = render(
      <TaskStateActions state="COMPLETED" foregroundActivity="background" />,
    );
    expect(container.querySelector(".tabler-icon-loader")).not.toBeNull();
    expect(container.querySelector(ICON_CHECK)).toBeNull();
    // Distinct by SHAPE from the generating spinner (IconLoader2), not hue alone.
    expect(container.querySelector(ICON_LOADER2)).toBeNull();
  });

  it("shows the generating spinner (IconLoader2) when any session is generating", () => {
    const { container } = render(
      <TaskStateActions state="COMPLETED" foregroundActivity="generating" />,
    );
    expect(container.querySelector(ICON_LOADER2)).not.toBeNull();
    expect(container.querySelector(ICON_CHECK)).toBeNull();
  });

  it("falls through to the coarse done check when no session is active", () => {
    const { container } = render(<TaskStateActions state="COMPLETED" foregroundActivity={null} />);
    expect(container.querySelector(ICON_CHECK)).not.toBeNull();
    expect(container.querySelector(ICON_LOADER2)).toBeNull();
  });

  // §spec:waiting-for-input-parity: the header status icon carries the sidebar's
  // rich waiting reading, distinct from done and running by SHAPE.
  it("shows the message-question for a pending clarification", () => {
    const { container } = render(
      <TaskStateActions state="IN_PROGRESS" hasPendingClarification foregroundActivity={null} />,
    );
    expect(container.querySelector(".tabler-icon-message-question")).not.toBeNull();
    expect(container.querySelector(ICON_CHECK)).toBeNull();
    expect(container.querySelector(ICON_LOADER2)).toBeNull();
  });

  it("shows the shield-question for a pending permission, distinct from done and running", () => {
    const { container } = render(
      <TaskStateActions state="WAITING_FOR_INPUT" hasPendingPermission foregroundActivity={null} />,
    );
    expect(container.querySelector(".tabler-icon-shield-question")).not.toBeNull();
    expect(container.querySelector(ICON_CHECK)).toBeNull();
    expect(container.querySelector(ICON_LOADER2)).toBeNull();
  });
});
