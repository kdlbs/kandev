import { describe, expect, it } from "vitest";
import { remarkTaskLinks, splitTaskIdentifierText } from "./task-identifier-link";

describe("task identifier links", () => {
  it("splits uppercase task identifiers into markdown link nodes", () => {
    expect(splitTaskIdentifierText("see KAN-42 and QA-7")).toEqual([
      { type: "text", value: "see " },
      {
        type: "link",
        url: "/office/tasks/KAN-42",
        children: [{ type: "text", value: "KAN-42" }],
      },
      { type: "text", value: " and " },
      {
        type: "link",
        url: "/office/tasks/QA-7",
        children: [{ type: "text", value: "QA-7" }],
      },
    ]);
  });

  it("leaves text without identifiers unchanged", () => {
    expect(splitTaskIdentifierText("kan-42 is lowercase")).toEqual([
      { type: "text", value: "kan-42 is lowercase" },
    ]);
  });

  it("does not rewrite text inside existing links", () => {
    const tree = {
      type: "root",
      children: [
        {
          type: "paragraph",
          children: [
            {
              type: "link",
              url: "/already",
              children: [{ type: "text", value: "KAN-42" }],
            },
          ],
        },
      ],
    };

    remarkTaskLinks()(tree);

    expect(tree.children[0].children).toEqual([
      {
        type: "link",
        url: "/already",
        children: [{ type: "text", value: "KAN-42" }],
      },
    ]);
  });
});
