import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { GoalFormFields } from "./goal-form-fields";
import { MayDoRows } from "./may-do-section";
import { WatchesFields } from "./watches-section";

afterEach(cleanup);

const actions = {
  create_task: "requires_approval",
  start_agent: "denied",
  message: "requires_approval",
  move: "requires_approval",
  resume: "requires_approval",
  stop: "denied",
} as const;

describe("MayDoRows without a stored coordinator", () => {
  it("keeps the rows and notes but no activity, review link or fresh-conversation note", () => {
    render(<MayDoRows actions={{ ...actions }} canManage onChange={vi.fn()} />);
    expect(screen.getAllByTestId(/^may-do-row-/)).toHaveLength(6);
    expect(screen.getByTestId("may-do-always-human")).toBeDefined();
    expect(screen.queryByTestId("may-do-review-create_task")).toBeNull();
    expect(screen.queryByText("Saving a change starts the next conversation fresh.")).toBeNull();
  });

  it("shows a row error beside its row and a policy line above the rows", () => {
    render(
      <MayDoRows
        actions={{ ...actions }}
        canManage
        onChange={vi.fn()}
        rowErrors={{ move: "Row problem" }}
        errorMessage="Policy problem"
      />,
    );
    expect(screen.getByTestId("may-do-error-move").textContent).toBe("Row problem");
    expect(screen.getByTestId("may-do-error").textContent).toBe("Policy problem");
  });

  it("reports a changed setting", () => {
    const onChange = vi.fn();
    render(<MayDoRows actions={{ ...actions }} canManage onChange={onChange} />);
    fireEvent.click(screen.getByLabelText("Denied", { selector: "#may-do-create_task-denied" }));
    expect(onChange).toHaveBeenCalledWith("create_task", "denied");
  });
});

describe("WatchesFields with an error line", () => {
  it("shows the message under the choice", () => {
    render(
      <WatchesFields
        watches={{ scope: "selected", workflowIds: [] }}
        boards={[{ id: "a", name: "Alpha", hidden: false }]}
        boardsStatus="ready"
        onBoardsRetry={vi.fn()}
        canManage
        onChange={vi.fn()}
        errorMessage="Pick a board"
      />,
    );
    expect(screen.getByTestId("watches-error").textContent).toBe("Pick a board");
  });

  it("keeps the switch off `selected` while the boards read has not succeeded", () => {
    render(
      <WatchesFields
        watches={{ scope: "all", workflowIds: [] }}
        boards={[]}
        boardsStatus="loading"
        onBoardsRetry={vi.fn()}
        canManage
        onChange={vi.fn()}
      />,
    );
    expect((screen.getByRole("switch") as HTMLButtonElement).disabled).toBe(true);
  });
});

describe("GoalFormFields in draft mode", () => {
  const form = {
    name: "Ship",
    dueOn: "",
    criteria: [{ key: "k", text: "One", done: false }],
  };

  it("has no done checkboxes and shows per-field and criteria-list messages", () => {
    render(
      <GoalFormFields
        draft
        form={form}
        disabled={false}
        fieldError={null}
        onChange={vi.fn()}
        messages={{ name: "Name problem", "criteria[0].text": "Text problem" }}
        criteriaMessage="List problem"
      />,
    );
    expect(screen.queryByRole("checkbox")).toBeNull();
    expect(screen.getByText("Name problem")).toBeDefined();
    expect(screen.getByText("Text problem")).toBeDefined();
    expect(screen.getByTestId("goal-criteria-error").textContent).toBe("List problem");
  });
});
