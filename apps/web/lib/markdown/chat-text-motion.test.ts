import { describe, expect, it } from "vitest";
import { appendedTextRuns } from "./chat-text-motion";

// @covers AC-UI-CHAT-MOTION-001.1, AC-UI-CHAT-MOTION-001.4
describe("incoming prose ranges", () => {
  it("animates only the appended suffix", () => {
    expect(appendedTextRuns("hello", "hello world", [], 10)).toEqual([
      { start: 5, end: 11, receivedAt: 10 },
    ]);
  });
  it("preserves active runs and expires settled ranges", () => {
    const runs = [{ start: 1, end: 2, receivedAt: 0 }];
    expect(appendedTextRuns("ab", "abc", runs, 20)).toEqual([
      ...runs,
      { start: 2, end: 3, receivedAt: 20 },
    ]);
    expect(appendedTextRuns("ab", "abc", runs, 200)).toEqual([
      { start: 2, end: 3, receivedAt: 200 },
    ]);
  });
  it("coalesces updates delivered in the same frame", () => {
    const runs = appendedTextRuns("a", "ab", [], 10);
    expect(appendedTextRuns("ab", "abc", runs, 12)).toEqual([{ start: 1, end: 3, receivedAt: 10 }]);
  });
  it("renders rewrites and truncation statically", () => {
    const runs = [{ start: 0, end: 3, receivedAt: 0 }];
    expect(appendedTextRuns("abc", "xyz", runs, 10)).toEqual([]);
    expect(appendedTextRuns("abc", "ab", runs, 10)).toEqual([]);
  });
  it("keeps UTF-16 boundaries aligned to the original Unicode text", () => {
    expect(appendedTextRuns("你好 👋", "你好 👋 世界", [], 1)).toEqual([
      { start: 5, end: 8, receivedAt: 1 },
    ]);
  });
});

import { splitMotionText } from "./chat-text-motion";
it("splits a Markdown text node without fading existing inline prose", () => {
  expect(
    splitMotionText("hello world", 2, "**hello world**", [{ start: 7, end: 13, receivedAt: 1 }]),
  ).toEqual([{ text: "hello" }, { text: " world", receivedAt: 1 }]);
});
it("leaves decoded entities and unpositioned nodes static", () => {
  const runs = [{ start: 0, end: 100, receivedAt: 1 }];
  expect(splitMotionText("a & b", 0, "a &amp; b", runs)).toEqual([{ text: "a & b" }]);
  expect(splitMotionText("text", undefined, "text", runs)).toEqual([{ text: "text" }]);
});
