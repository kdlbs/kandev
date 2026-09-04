import { describe, expect, it } from "vitest";
import {
  DEFAULT_THREAD_VIEW,
  createDefaultThreadView,
  threadViewName,
} from "./thread-view-builtins";

describe("default thread view", () => {
  it("starts the canonical and newly created views with five columns", () => {
    expect(DEFAULT_THREAD_VIEW.maxColumns).toBe(5);
    expect(createDefaultThreadView("view-new", "New view").maxColumns).toBe(5);
  });
});

describe("threadViewName", () => {
  it("translates the unrenamed built-in view", () => {
    expect(threadViewName(DEFAULT_THREAD_VIEW, (key) => `translated:${key}`)).toBe(
      "translated:threads:allThreads",
    );
  });

  it("preserves a custom view name", () => {
    expect(
      threadViewName({ id: DEFAULT_THREAD_VIEW.id, name: "My threads" }, () => "translated"),
    ).toBe("My threads");
  });
});
