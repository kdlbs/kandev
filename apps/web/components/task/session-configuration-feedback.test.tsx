import { act, cleanup, render, renderHook } from "@testing-library/react";
import { createRef } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";

import { ApiError } from "@/lib/api/client";

import {
  PickerConfigurationFeedbackMessage,
  usePickerConfigurationFeedback,
} from "./session-configuration-feedback";

const SESSION_A = "session-a";
const SESSION_B = "session-b";
const EXECUTION_1 = "execution-1";
const EXECUTION_2 = "execution-2";
const MODEL_B = "model-b";
const SAVE_PERSISTENCE_ERROR = "failed to persist session runtime config";

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

describe("picker configuration outcome summaries", () => {
  it("shows a deferred persistence failure without implying the setting was applied", () => {
    const anchorRef = createRef<HTMLDivElement>();
    const { result } = renderHook(() =>
      usePickerConfigurationFeedback(false, {
        sessionId: SESSION_A,
        executionId: EXECUTION_1,
      }),
    );
    act(() => {
      const id = result.current.controller.begin();
      result.current.controller.showResult(id, "Model", MODEL_B, {
        status: "persistence_failed",
        requested: MODEL_B,
        confirmed: false,
      });
    });

    expect(result.current.feedback?.status).toBe("persistence_failed");
    const view = render(
      <PickerConfigurationFeedbackMessage
        anchorRef={anchorRef}
        isMobile={false}
        message={result.current.message}
        passive={result.current.passive}
      />,
    );
    expect(view.container.textContent).toContain("could not be saved");
    expect(view.container.textContent).not.toContain("applied");
  });

  it("shows the confirmed value and the unconfirmed option in a mixed result", () => {
    const anchorRef = createRef<HTMLDivElement>();
    const { result } = renderHook(() =>
      usePickerConfigurationFeedback(false, {
        sessionId: SESSION_A,
        executionId: EXECUTION_1,
      }),
    );
    act(() => {
      const id = result.current.controller.begin();
      result.current.controller.showChanges(id, [
        {
          label: "Model",
          requested: MODEL_B,
          result: {
            status: "applied",
            requested: MODEL_B,
            effective: MODEL_B,
            confirmed: true,
            execution_id: EXECUTION_1,
          },
        },
        {
          label: "Thinking",
          requested: "high",
          result: { status: "unconfirmed", requested: "high", confirmed: false },
        },
      ]);
    });

    expect(result.current.feedback?.status).toBe("mixed");
    const view = render(
      <PickerConfigurationFeedbackMessage
        anchorRef={anchorRef}
        isMobile={false}
        message={result.current.message}
        passive={result.current.passive}
      />,
    );
    expect(view.container.textContent).toContain(`Model: ${MODEL_B}`);
    expect(view.container.textContent).toContain("Thinking: high");
  });
});

describe("picker partial persistence feedback", () => {
  it("keeps a live-but-unsaved base result visible when a later option is rejected", () => {
    const anchorRef = createRef<HTMLDivElement>();
    const { result } = renderHook(() =>
      usePickerConfigurationFeedback(false, {
        sessionId: SESSION_A,
        executionId: EXECUTION_1,
      }),
    );
    const modelSaveError = new ApiError(SAVE_PERSISTENCE_ERROR, 500, {
      error: SAVE_PERSISTENCE_ERROR,
      configuration_result: {
        status: "persistence_failed",
        config_id: "model",
        requested: MODEL_B,
        effective: MODEL_B,
        confirmed: true,
        execution_id: EXECUTION_1,
      },
    });
    const optionRejection = new ApiError("requested value is unsupported", 500, {
      error: "requested value is unsupported",
      configuration_result: {
        status: "rejected",
        config_id: "thinking",
        requested: "high",
        confirmed: false,
        execution_id: EXECUTION_1,
      },
    });
    act(() => {
      const id = result.current.controller.begin();
      result.current.controller.showChanges(
        id,
        [
          {
            label: "Model",
            requested: MODEL_B,
            result:
              modelSaveError.body &&
              (modelSaveError.body as { configuration_result: never }).configuration_result,
            error: modelSaveError,
          } as never,
          {
            label: "Thinking",
            requested: "high",
            result: (optionRejection.body as { configuration_result: never }).configuration_result,
            error: optionRejection,
          } as never,
        ],
        { label: "Thinking", value: "high", error: optionRejection },
      );
    });

    const view = render(
      <PickerConfigurationFeedbackMessage
        anchorRef={anchorRef}
        isMobile={false}
        message={result.current.message}
        passive={result.current.passive}
      />,
    );
    expect(view.container.textContent).toContain(`Model: ${MODEL_B} applied`);
    expect(view.container.textContent).toContain("could not be saved");
    expect(view.container.textContent).toContain("Thinking: high");
    expect(view.container.textContent).toContain("requested value is unsupported");
  });
});
