import { describe, expect, it } from "vitest";
import { remarkTaskLinks, splitTaskIdentifierText } from "./task-identifier-link";

describe("task identifier links", () => {
  it("links only identifiers carrying the workspace prefix", () => {
    expect(splitTaskIdentifierText("see KAN-11 and VG-30375", "KAN")).toEqual([
      { type: "text", value: "see " },
      {
        type: "link",
        url: "/office/tasks/KAN-11",
        children: [{ type: "text", value: "KAN-11" }],
      },
      { type: "text", value: " and VG-30375" },
    ]);
  });

  it("carries the workspace in the link when given", () => {
    expect(splitTaskIdentifierText("KAN-11", "KAN", "ws 1")).toEqual([
      {
        type: "link",
        url: "/office/tasks/KAN-11?workspace=ws%201",
        children: [{ type: "text", value: "KAN-11" }],
      },
    ]);
  });

  it("leaves external tracker keys as plain text", () => {
    expect(splitTaskIdentifierText("VG-30375 and NTTVS-618", "KAN")).toEqual([
      { type: "text", value: "VG-30375 and NTTVS-618" },
    ]);
  });

  it("does not match a longer prefix ending in the workspace prefix", () => {
    expect(splitTaskIdentifierText("XKAN-1", "KAN")).toEqual([{ type: "text", value: "XKAN-1" }]);
  });

  it("links nothing without a workspace prefix", () => {
    expect(splitTaskIdentifierText("see KAN-42", undefined)).toEqual([
      { type: "text", value: "see KAN-42" },
    ]);
  });

  it("leaves text without identifiers unchanged", () => {
    expect(splitTaskIdentifierText("kan-42 is lowercase", "KAN")).toEqual([
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

    remarkTaskLinks({ prefix: "KAN" })(tree);

    expect(tree.children[0].children).toEqual([
      {
        type: "link",
        url: "/already",
        children: [{ type: "text", value: "KAN-42" }],
      },
    ]);
  });

  it("does not rewrite text inside code", () => {
    const tree = {
      type: "root",
      children: [{ type: "paragraph", children: [{ type: "inlineCode", value: "KAN-42" }] }],
    };

    remarkTaskLinks({ prefix: "KAN" })(tree);

    expect(tree.children[0].children).toEqual([{ type: "inlineCode", value: "KAN-42" }]);
  });
});
