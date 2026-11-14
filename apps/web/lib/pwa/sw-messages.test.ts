import { describe, expect, it } from "vitest";
import { SW_NAVIGATE_MESSAGE, navigationTargetFromMessage } from "./sw-messages";
import { SW_MESSAGE_NAVIGATE } from "@/sw/sw-core";

describe("navigationTargetFromMessage", () => {
  it("uses the same message type as the worker", () => {
    expect(SW_NAVIGATE_MESSAGE).toBe(SW_MESSAGE_NAVIGATE);
  });

  it("returns the in-app path from a navigate message", () => {
    expect(
      navigationTargetFromMessage({ type: SW_NAVIGATE_MESSAGE, url: "/t/1?sessionId=s" }),
    ).toBe("/t/1?sessionId=s");
  });

  it("ignores other messages and unsafe targets", () => {
    expect(navigationTargetFromMessage(null)).toBeNull();
    expect(navigationTargetFromMessage("x")).toBeNull();
    expect(navigationTargetFromMessage({ type: "other", url: "/t/1" })).toBeNull();
    expect(navigationTargetFromMessage({ type: SW_NAVIGATE_MESSAGE })).toBeNull();
    for (const url of ["https://evil.example", "//evil.example", "t/1", "/\\x"]) {
      expect(navigationTargetFromMessage({ type: SW_NAVIGATE_MESSAGE, url })).toBeNull();
    }
  });
});
