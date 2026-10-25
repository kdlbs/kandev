import { act, renderHook } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import { useThreadFocusRequest } from "./use-thread-focus-request";

// @covers AC-TASKS-THREADS-ACTIONS-003.4
describe("thread focus request lifetime", () => {
  it("activates a requested target that resolves after hydration", () => {
    const { result, rerender } = renderHook(
      ({ target }: { target: string | null }) => useThreadFocusRequest(target, "request-b"),
      { initialProps: { target: null as string | null } },
    );

    rerender({ target: "b" });

    expect(result.current).toMatchObject({ markedTaskId: "b", activationTaskId: "b" });
  });

  it("does not activate a late target after the reader has consumed its request", () => {
    const { result, rerender } = renderHook(
      ({ target }: { target: string | null }) => useThreadFocusRequest(target, "request-b"),
      { initialProps: { target: null as string | null } },
    );
    act(() => result.current.retire());

    rerender({ target: "b" });

    expect(result.current).toMatchObject({ markedTaskId: null, activationTaskId: null });
  });

  it("rearms only a new request after its consumed target departs and returns", () => {
    const { result, rerender } = renderHook(
      ({ target, request }: { target: string | null; request: string }) =>
        useThreadFocusRequest(target, request),
      { initialProps: { target: "b" as string | null, request: "request-b" } },
    );
    act(() => result.current.retire());
    expect(result.current).toMatchObject({ markedTaskId: null, activationTaskId: "b" });
    rerender({ target: null, request: "request-b" });
    rerender({ target: "b", request: "request-b" });
    expect(result.current).toMatchObject({ markedTaskId: null, activationTaskId: null });

    rerender({ target: "b", request: "new-request-b" });

    expect(result.current).toMatchObject({ markedTaskId: "b", activationTaskId: "b" });
  });
});
