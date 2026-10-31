import { act, cleanup, renderHook } from "@testing-library/react";
import type { KeyboardEvent, PointerEvent } from "react";
import { afterEach, expect, it, vi } from "vitest";
import { useReviewDiffScrollInteraction } from "./use-review-diff-scroll-interaction";
import type { ReviewFile } from "./types";

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

it.each([
  ["wheel", undefined],
  ["key", undefined],
  ["scrollbar", undefined],
  ["child pointer", "mouse"],
  ["touch child pointer", "touch"],
  ["pen child pointer", "pen"],
  ["other key", undefined],
] as const)("applies the %s input policy while a restore is pending", (input, pointerType) => {
  const frames = new Map<number, FrameRequestCallback>();
  let next = 0;
  vi.stubGlobal("requestAnimationFrame", (callback: FrameRequestCallback) => {
    frames.set(++next, callback);
    return next;
  });
  vi.stubGlobal("cancelAnimationFrame", (id: number) => frames.delete(id));
  const root = document.createElement("div");
  const section = document.createElement("section");
  section.dataset.reviewFileKey = "a.ts";
  root.append(section);
  for (const element of [root, section]) {
    element.getBoundingClientRect = () => ({
      top: 0,
      bottom: 500,
      left: 0,
      right: 300,
      width: 300,
      height: 500,
      x: 0,
      y: 0,
      toJSON() {},
    });
  }
  const rootRef = { current: root };
  const file: ReviewFile = {
    path: "a.ts",
    diff: "old",
    status: "modified",
    additions: 1,
    deletions: 1,
    staged: false,
    source: "uncommitted",
  };
  const { result, rerender } = renderHook(
    ({ diff }) =>
      useReviewDiffScrollInteraction({
        rootRef,
        files: [{ ...file, diff }],
        sessionId: "session-1",
        sourceKey: "uncommitted",
      }),
    { initialProps: { diff: "old" } },
  );
  act(() => rerender({ diff: "new" }));
  expect(frames.size).toBe(1);
  expect(result.current.suppressAutoMarkRef.current).toBe(true);
  act(() => {
    if (input === "wheel") result.current.allowAutoMark();
    if (input === "key")
      result.current.handleKeyDown({ key: "ArrowDown" } as KeyboardEvent<HTMLDivElement>);
    if (input === "scrollbar")
      result.current.handlePointerDown({
        target: root,
        currentTarget: root,
      } as unknown as PointerEvent<HTMLDivElement>);
    if (pointerType)
      result.current.handlePointerDown({
        target: section,
        currentTarget: root,
        pointerType,
      } as unknown as PointerEvent<HTMLDivElement>);
    if (input === "other key")
      result.current.handleKeyDown({ key: "a" } as KeyboardEvent<HTMLDivElement>);
  });
  expect(frames.size).toBe(input === "other key" ? 1 : 0);
  expect(result.current.suppressAutoMarkRef.current).toBe(
    ["child pointer", "pen child pointer", "other key"].includes(input),
  );
});
