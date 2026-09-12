import { cleanup, renderHook } from "@testing-library/react";
import { useLayoutEffect } from "react";
import { afterEach, expect, it } from "vitest";
import { useMobileThreadPosition } from "./use-mobile-thread-position";

afterEach(cleanup);

it("uses the current fallback on the first phone commit after leaving desktop", () => {
  const boardRef = { current: document.createElement("div") as HTMLDivElement | null };
  const elementsRef = { current: new Map<string, HTMLElement>() };
  const committed: Array<string | null> = [];
  const { result, rerender } = renderHook(
    ({ enabled, fallbackTaskId }) => {
      const taskId = useMobileThreadPosition({
        enabled,
        fallbackTaskId,
        boardRef,
        elementsRef,
        orderedIds: ["a", "b"],
      });
      useLayoutEffect(() => {
        committed.push(taskId);
      });
      return taskId;
    },
    { initialProps: { enabled: true, fallbackTaskId: "a" } },
  );
  expect(result.current).toBe("a");
  rerender({ enabled: false, fallbackTaskId: "b" });
  expect(result.current).toBeNull();
  boardRef.current = null;
  committed.length = 0;
  rerender({ enabled: true, fallbackTaskId: "b" });
  expect(committed[0]).toBe("b");
  expect(result.current).toBe("b");
});
