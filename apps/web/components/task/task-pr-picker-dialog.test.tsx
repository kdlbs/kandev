import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { useState } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { TaskPR } from "@/lib/types/github";
import type { TaskMR } from "@/lib/types/gitlab";
import type { TaskReviewTarget } from "./task-pr-open";
import { TaskPRPickerDialog } from "./task-pr-picker-dialog";

const targets: TaskReviewTarget[] = [
  {
    type: "pr",
    key: "pr:1",
    url: "https://github.com/acme/kandev/pull/1",
    review: {
      id: "pr-1",
      repo: "kandev",
      pr_number: 1,
      pr_title: "GitHub review",
      state: "open",
      checks_state: "",
      checks_total: 0,
      checks_passing: 0,
      review_state: "",
      mergeable_state: "",
      review_count: 0,
      pending_review_count: 0,
    } as TaskPR,
  },
  {
    type: "mr",
    key: "mr:2",
    url: "https://gitlab.example/acme/kandev/-/merge_requests/2",
    review: {
      id: "mr-2",
      project_path: "acme/kandev",
      mr_iid: 2,
      mr_title: "GitLab review",
      mr_url: "https://gitlab.example/acme/kandev/-/merge_requests/2",
      state: "opened",
    } as TaskMR,
  },
];
const selectedAttribute = "data-selected";

function ControlledPicker({ onActivateIndex }: { onActivateIndex: (index: number) => void }) {
  const [selectedIndex, setSelectedIndex] = useState(0);

  return (
    <TaskPRPickerDialog
      open
      onOpenChange={() => undefined}
      targets={targets}
      selectedIndex={selectedIndex}
      onSelectedIndexChange={setSelectedIndex}
      onActivateIndex={onActivateIndex}
    />
  );
}

describe("TaskPRPickerDialog", () => {
  afterEach(() => cleanup());

  it("keeps controlled selection, focus, arrow keys, Enter, and click in sync", async () => {
    const onActivateIndex = vi.fn();
    render(<ControlledPicker onActivateIndex={onActivateIndex} />);

    const prRow = screen.getByTestId("task-pr-picker-row-pr-1");
    const mrRow = screen.getByTestId("task-mr-picker-row-mr-2");

    expect(prRow.compareDocumentPosition(mrRow)).toBe(Node.DOCUMENT_POSITION_FOLLOWING);
    await waitFor(() => expect(document.activeElement).toBe(prRow));
    expect(prRow.getAttribute(selectedAttribute)).toBe("true");

    fireEvent.keyDown(prRow, { key: "ArrowDown" });
    await waitFor(() => expect(document.activeElement).toBe(mrRow));
    expect(mrRow.getAttribute(selectedAttribute)).toBe("true");

    fireEvent.keyDown(mrRow, { key: "ArrowDown" });
    await waitFor(() => expect(document.activeElement).toBe(prRow));
    expect(prRow.getAttribute(selectedAttribute)).toBe("true");

    fireEvent.keyDown(prRow, { key: "ArrowUp" });
    await waitFor(() => expect(document.activeElement).toBe(mrRow));

    fireEvent.keyDown(mrRow, { key: "Enter" });
    fireEvent.click(prRow);

    expect(onActivateIndex).toHaveBeenNthCalledWith(1, 1);
    expect(onActivateIndex).toHaveBeenNthCalledWith(2, 0);
  });

  it("activates the row focused with Tab when Enter is pressed", async () => {
    const onActivateIndex = vi.fn();
    render(<ControlledPicker onActivateIndex={onActivateIndex} />);

    const prRow = screen.getByTestId("task-pr-picker-row-pr-1");
    const mrRow = screen.getByTestId("task-mr-picker-row-mr-2");
    await waitFor(() => expect(document.activeElement).toBe(prRow));

    mrRow.focus();
    expect(document.activeElement).toBe(mrRow);
    await waitFor(() => expect(mrRow.getAttribute(selectedAttribute)).toBe("true"));
    fireEvent.keyDown(mrRow, { key: "Enter" });

    expect(onActivateIndex).toHaveBeenCalledWith(1);
  });
});
