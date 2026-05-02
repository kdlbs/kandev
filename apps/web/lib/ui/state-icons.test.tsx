import { describe, expect, it } from "vitest";
import { isValidElement, type ReactNode } from "react";
import { IconCheck, IconMessageQuestion } from "@tabler/icons-react";
import { getTaskStateIcon } from "./state-icons";

function iconType(node: ReactNode) {
  if (!isValidElement(node)) throw new Error("Expected React element");
  return node.type;
}

describe("getTaskStateIcon", () => {
  it("uses the question icon for waiting-for-input task state", () => {
    expect(iconType(getTaskStateIcon("WAITING_FOR_INPUT"))).toBe(IconMessageQuestion);
  });

  it("uses the question icon when there is a pending clarification", () => {
    expect(iconType(getTaskStateIcon("REVIEW", undefined, true))).toBe(IconMessageQuestion);
  });

  it("keeps review task state as the review check without pending clarification", () => {
    expect(iconType(getTaskStateIcon("REVIEW", undefined, false))).toBe(IconCheck);
  });
});
