import { describe, expect, it } from "vitest";
import { groupAzureDevOpsBoardItems } from "./azure-devops-board-view";

describe("groupAzureDevOpsBoardItems", () => {
  it("preserves board column order and omits cards in unknown columns", () => {
    const board = {
      id: "board-1",
      name: "Stories",
      fields: {
        columnField: { referenceName: "" },
        doneField: { referenceName: "" },
        rowField: { referenceName: "" },
      },
      columns: [
        { id: "todo", name: "To Do" },
        { id: "done", name: "Done" },
      ],
    };
    const groups = groupAzureDevOpsBoardItems(board, [
      {
        id: 2,
        revision: 1,
        title: "Done",
        state: "",
        type: "Bug",
        columnId: "done",
        columnDone: true,
      },
      {
        id: 1,
        revision: 1,
        title: "Todo",
        state: "",
        type: "Task",
        columnId: "todo",
        columnDone: false,
      },
      {
        id: 3,
        revision: 1,
        title: "Hidden",
        state: "",
        type: "Task",
        columnId: "other",
        columnDone: false,
      },
    ]);
    expect([...groups.keys()]).toEqual(["todo", "done"]);
    expect(groups.get("todo")?.map((item) => item.id)).toEqual([1]);
    expect(groups.get("done")?.map((item) => item.id)).toEqual([2]);
  });
});
