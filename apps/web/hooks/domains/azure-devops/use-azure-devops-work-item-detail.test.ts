import { describe, expect, it } from "vitest";
import { mergeAzureDevOpsComments } from "./use-azure-devops-work-item-detail";

describe("mergeAzureDevOpsComments", () => {
  it("appends older pages without duplicating comments", () => {
    const current = [
      { id: 3, content: "new", author: { id: "u", displayName: "Ada" } },
      { id: 2, content: "middle", author: { id: "u", displayName: "Ada" } },
    ];
    const older = [
      { id: 2, content: "middle", author: { id: "u", displayName: "Ada" } },
      { id: 1, content: "old", author: { id: "u", displayName: "Ada" } },
    ];
    expect(mergeAzureDevOpsComments(current, older).map((comment) => comment.id)).toEqual([
      3, 2, 1,
    ]);
  });
});
