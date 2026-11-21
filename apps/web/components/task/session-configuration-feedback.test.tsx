import { act, cleanup, renderHook } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";

import { usePickerConfigurationFeedback } from "./session-configuration-feedback";

const SESSION_A = "session-a";
const SESSION_B = "session-b";
const EXECUTION_1 = "execution-1";
const EXECUTION_2 = "execution-2";
const MODEL_B = "model-b";

afterEach(() => {
  cleanup();
  vi.useRealTimers();
});

describe("picker configuration feedback", () => {
  it("expires a confirmed result after one visible second and pauses while the picker is open", () => {
    vi.useFakeTimers();
    const { result, rerender } = renderHook(
      ({ open }) =>
        usePickerConfigurationFeedback(open, {
          sessionId: SESSION_A,
          executionId: EXECUTION_1,
        }),
      { initialProps: { open: true } },
    );

    act(() => {
      const id = result.current.controller.begin();
      result.current.controller.showResult(id, "Model", MODEL_B, {
        status: "applied",
        requested: MODEL_B,
        effective: MODEL_B,
        confirmed: true,
        execution_id: EXECUTION_1,
      });
    });

    expect(result.current.feedback?.status).toBe("applied");
    act(() => vi.advanceTimersByTime(1500));
    expect(result.current.feedback?.status).toBe("applied");

    rerender({ open: false });
    act(() => vi.advanceTimersByTime(999));
    expect(result.current.feedback?.status).toBe("applied");
    act(() => vi.advanceTimersByTime(1));
    expect(result.current.feedback).toBeUndefined();
  });

  it("expires after only the remaining visible time when the picker is reopened", () => {
    vi.useFakeTimers();
    const { result, rerender } = renderHook(
      ({ open }) =>
        usePickerConfigurationFeedback(open, {
          sessionId: SESSION_A,
          executionId: EXECUTION_1,
        }),
      { initialProps: { open: false } },
    );

    act(() => {
      const id = result.current.controller.begin();
      result.current.controller.showResult(id, "Model", MODEL_B, {
        status: "applied",
        requested: MODEL_B,
        effective: MODEL_B,
        confirmed: true,
        execution_id: EXECUTION_1,
      });
    });
    act(() => vi.advanceTimersByTime(400));
    rerender({ open: true });
    act(() => vi.advanceTimersByTime(1_000));
    rerender({ open: false });
    act(() => vi.advanceTimersByTime(599));
    expect(result.current.feedback?.status).toBe("applied");
    act(() => vi.advanceTimersByTime(1));
    expect(result.current.feedback).toBeUndefined();
  });

  it("ignores an obsolete result after a newer picker action", () => {
    const { result } = renderHook(() => usePickerConfigurationFeedback(false));
    let firstId = 0;
    act(() => {
      firstId = result.current.controller.begin();
      result.current.controller.begin();
      result.current.controller.showResult(firstId, "Model", "old", {
        status: "applied",
        requested: "old",
        effective: "old",
        confirmed: true,
      });
    });
    expect(result.current.feedback).toBeUndefined();
  });

  it("shows uncertainty as a dismissible result", () => {
    const { result } = renderHook(() => usePickerConfigurationFeedback(false));
    act(() => {
      const id = result.current.controller.begin();
      result.current.controller.showResult(id, "Model", "model-b");
    });
    expect(result.current.feedback?.status).toBe("unconfirmed");
    act(() => result.current.dismiss());
    expect(result.current.feedback).toBeUndefined();
  });
});

describe("picker configuration feedback scope", () => {
  it("does not let a delayed success or failure cross a session or execution change", () => {
    const { result, rerender } = renderHook(
      ({ scope }) => usePickerConfigurationFeedback(false, scope),
      { initialProps: { scope: { sessionId: SESSION_A, executionId: EXECUTION_1 } } },
    );
    const oldSessionController = result.current.controller;
    let oldSessionRequest = 0;
    act(() => {
      oldSessionRequest = oldSessionController.begin();
    });

    rerender({ scope: { sessionId: SESSION_B, executionId: EXECUTION_1 } });
    act(() => {
      oldSessionController.showResult(oldSessionRequest, "Model", "old-model", {
        status: "applied",
        requested: "old-model",
        effective: "old-model",
        confirmed: true,
        execution_id: EXECUTION_1,
      });
    });
    expect(result.current.feedback).toBeUndefined();

    const oldExecutionController = result.current.controller;
    let oldExecutionRequest = 0;
    act(() => {
      oldExecutionRequest = oldExecutionController.begin();
    });
    rerender({ scope: { sessionId: SESSION_B, executionId: EXECUTION_2 } });
    act(() => {
      oldExecutionController.showError(
        oldExecutionRequest,
        "Model",
        "old-model",
        new Error("old execution failed"),
      );
    });
    expect(result.current.feedback).toBeUndefined();
  });

  it("requires a matching execution identity for live confirmation", () => {
    const { result } = renderHook(() =>
      usePickerConfigurationFeedback(false, {
        sessionId: "session-a",
        executionId: EXECUTION_2,
      }),
    );
    act(() => {
      const id = result.current.controller.begin();
      result.current.controller.showResult(id, "Model", MODEL_B, {
        status: "applied",
        requested: MODEL_B,
        effective: MODEL_B,
        confirmed: true,
        execution_id: EXECUTION_1,
      });
    });
    expect(result.current.feedback).toBeUndefined();

    act(() => {
      const id = result.current.controller.begin();
      result.current.controller.showResult(id, "Model", MODEL_B, {
        status: "applied",
        requested: MODEL_B,
        effective: MODEL_B,
        confirmed: true,
      });
    });
    expect(result.current.feedback?.status).toBe("unconfirmed");
  });

  it("keeps transport failures unconfirmed", () => {
    const { result } = renderHook(() => usePickerConfigurationFeedback(false));
    act(() => {
      const id = result.current.controller.begin();
      result.current.controller.showError(id, "Model", "model-b", new Error("request timed out"));
    });
    expect(result.current.feedback?.status).toBe("unconfirmed");
  });
});
