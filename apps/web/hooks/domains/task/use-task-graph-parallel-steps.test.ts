import { act, renderHook } from "@testing-library/react";
import { beforeEach, describe, expect, it } from "vitest";
import {
  PARALLEL_STEPS_STORAGE_KEY,
  useTaskGraphParallelSteps,
} from "./use-task-graph-parallel-steps";

beforeEach(() => window.localStorage.clear());

describe("useTaskGraphParallelSteps", () => {
  it("defaults off and remembers the choice on this device", () => {
    const { result } = renderHook(() => useTaskGraphParallelSteps());
    expect(result.current[0]).toBe(false);
    act(() => result.current[1](true));
    expect(result.current[0]).toBe(true);
    expect(window.localStorage.getItem(PARALLEL_STEPS_STORAGE_KEY)).toBe("true");
    expect(renderHook(() => useTaskGraphParallelSteps()).result.current[0]).toBe(true);
  });
});
