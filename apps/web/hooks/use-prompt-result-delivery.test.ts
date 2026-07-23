import { act, renderHook } from "@testing-library/react";
import { beforeEach, expect, it, vi } from "vitest";

import type { UtilityGenerationResult } from "./use-utility-agent-generator";
import { usePromptResultDelivery } from "./use-prompt-result-delivery";

const mockToast = vi.fn();

vi.mock("@/components/toast-provider", () => ({
  useToast: () => ({ toast: mockToast }),
}));

const GENERATED_RESULT = {
  content: "enhanced prompt output",
  callId: "call-123",
  durationMs: 1_200,
} satisfies UtilityGenerationResult;

const INSERT_FAILURE_MESSAGE = "Enhanced prompt was generated but could not be inserted.";

beforeEach(() => {
  vi.clearAllMocks();
  Object.defineProperty(navigator, "clipboard", {
    configurable: true,
    value: { writeText: vi.fn().mockResolvedValue(undefined) },
  });
});

it.each([
  ["original", "original", true, "applies unchanged input"],
  ["original", "edited", false, "retains result after user edit"],
  ["original", null, false, "retains result after target disappears"],
])("%s", (source, current, expectedApplied, _label) => {
  const apply = vi.fn(() => true);
  const { result } = renderHook(() =>
    usePromptResultDelivery({ scopeKey: "test", getCurrent: () => current, apply }),
  );

  let delivered = false;
  act(() => {
    delivered = result.current.deliver(source, GENERATED_RESULT, result.current.captureScope());
  });
  expect(delivered).toBe(expectedApplied);
  expect(apply).toHaveBeenCalledTimes(expectedApplied ? 1 : 0);

  if (expectedApplied) {
    expect(apply).toHaveBeenCalledWith(GENERATED_RESULT.content);
    expect(result.current.pendingResult).toBeNull();
    expect(mockToast).not.toHaveBeenCalled();
    return;
  }

  expect(result.current.pendingResult).toEqual(GENERATED_RESULT);
  expect(mockToast).toHaveBeenCalledWith(
    expect.objectContaining({ description: INSERT_FAILURE_MESSAGE, variant: "error" }),
  );
});

it("retains the result when insertion rejects unchanged input", () => {
  const apply = vi.fn(() => false);
  const { result } = renderHook(() =>
    usePromptResultDelivery({ scopeKey: "test", getCurrent: () => "original", apply }),
  );

  let delivered = true;
  act(() => {
    delivered = result.current.deliver("original", GENERATED_RESULT, result.current.captureScope());
  });
  expect(delivered).toBe(false);

  expect(apply).toHaveBeenCalledWith(GENERATED_RESULT.content);
  expect(result.current.pendingResult).toEqual(GENERATED_RESULT);
  expect(mockToast).toHaveBeenCalledWith(
    expect.objectContaining({ description: INSERT_FAILURE_MESSAGE, variant: "error" }),
  );
});

it("applyPending clears only after apply succeeds", () => {
  const apply = vi.fn(() => false);
  const { result } = renderHook(() =>
    usePromptResultDelivery({ scopeKey: "test", getCurrent: () => "edited", apply }),
  );

  act(() => {
    result.current.deliver("original", GENERATED_RESULT, result.current.captureScope());
  });
  vi.clearAllMocks();

  act(() => {
    result.current.applyPending();
  });
  expect(apply).toHaveBeenCalledWith(GENERATED_RESULT.content);
  expect(result.current.pendingResult).toEqual(GENERATED_RESULT);

  apply.mockReturnValue(true);
  act(() => {
    result.current.applyPending();
  });
  expect(apply).toHaveBeenCalledTimes(2);
  expect(result.current.pendingResult).toBeNull();
});

it("copyPending writes the pending result and reports success", async () => {
  const writeText = vi.fn().mockResolvedValue(undefined);
  Object.defineProperty(navigator, "clipboard", {
    configurable: true,
    value: { writeText },
  });
  const { result } = renderHook(() =>
    usePromptResultDelivery({
      scopeKey: "test",
      getCurrent: () => "edited",
      apply: vi.fn(() => true),
    }),
  );

  act(() => {
    result.current.deliver("original", GENERATED_RESULT, result.current.captureScope());
  });
  vi.clearAllMocks();

  await act(async () => {
    await result.current.copyPending();
  });

  expect(writeText).toHaveBeenCalledWith(GENERATED_RESULT.content);
  expect(mockToast).toHaveBeenCalledWith(
    expect.objectContaining({
      description: "Enhanced prompt copied to clipboard.",
      variant: "success",
    }),
  );
  expect(result.current.pendingResult).toEqual(GENERATED_RESULT);
});

it("copyPending reports clipboard failure without clearing the result", async () => {
  const writeText = vi.fn().mockRejectedValue(new Error("denied"));
  Object.defineProperty(navigator, "clipboard", {
    configurable: true,
    value: { writeText },
  });
  const appendChild = vi.spyOn(document.body, "appendChild");
  const createElement = vi.spyOn(document, "createElement");
  const { result } = renderHook(() =>
    usePromptResultDelivery({
      scopeKey: "test",
      getCurrent: () => "edited",
      apply: vi.fn(() => true),
    }),
  );

  act(() => {
    result.current.deliver("original", GENERATED_RESULT, result.current.captureScope());
  });
  vi.clearAllMocks();

  await act(async () => {
    await result.current.copyPending();
  });

  expect(writeText).toHaveBeenCalledWith(GENERATED_RESULT.content);
  expect(mockToast).toHaveBeenCalledWith(
    expect.objectContaining({
      description: "Enhanced prompt could not be copied.",
      variant: "error",
    }),
  );
  expect(result.current.pendingResult).toEqual(GENERATED_RESULT);
  expect(appendChild).not.toHaveBeenCalled();
  expect(createElement).not.toHaveBeenCalledWith("textarea");
});

it("copyPending reports failure without DOM fallback when clipboard is unavailable", async () => {
  Object.defineProperty(navigator, "clipboard", {
    configurable: true,
    value: undefined,
  });
  const appendChild = vi.spyOn(document.body, "appendChild");
  const createElement = vi.spyOn(document, "createElement");
  const { result } = renderHook(() =>
    usePromptResultDelivery({
      scopeKey: "test",
      getCurrent: () => "edited",
      apply: vi.fn(() => true),
    }),
  );

  act(() => {
    result.current.deliver("original", GENERATED_RESULT, result.current.captureScope());
  });
  vi.clearAllMocks();

  await act(async () => {
    await result.current.copyPending();
  });

  expect(mockToast).toHaveBeenCalledWith(
    expect.objectContaining({
      description: "Enhanced prompt could not be copied.",
      variant: "error",
    }),
  );
  expect(result.current.pendingResult).toEqual(GENERATED_RESULT);
  expect(appendChild).not.toHaveBeenCalled();
  expect(createElement).not.toHaveBeenCalledWith("textarea");
});

it("dismissPending clears the retained result", () => {
  const { result } = renderHook(() =>
    usePromptResultDelivery({
      scopeKey: "test",
      getCurrent: () => "edited",
      apply: vi.fn(() => true),
    }),
  );

  act(() => {
    result.current.deliver("original", GENERATED_RESULT, result.current.captureScope());
  });

  expect(result.current.pendingResult).toEqual(GENERATED_RESULT);

  act(() => {
    result.current.dismissPending();
  });

  expect(result.current.pendingResult).toBeNull();
});

it("ignores a delayed result after the dialog closes and reopens with the same text", () => {
  const apply = vi.fn(() => true);
  const { result, rerender } = renderHook(
    ({ scopeKey }) => usePromptResultDelivery({ scopeKey, getCurrent: () => "original", apply }),
    { initialProps: { scopeKey: "dialog:task-1:open-1" } },
  );
  const generation = result.current.captureScope();

  rerender({ scopeKey: "dialog:task-1:open-2" });

  let delivered = true;
  act(() => {
    delivered = result.current.deliver("original", GENERATED_RESULT, generation);
  });

  expect(delivered).toBe(false);
  expect(apply).not.toHaveBeenCalled();
  expect(result.current.pendingResult).toBeNull();
  expect(mockToast).not.toHaveBeenCalled();
});

it("ignores a delayed result after switching task or session with the same text", () => {
  const apply = vi.fn(() => true);
  const { result, rerender } = renderHook(
    ({ scopeKey }) => usePromptResultDelivery({ scopeKey, getCurrent: () => "original", apply }),
    { initialProps: { scopeKey: "task-1:session-1" } },
  );
  const generation = result.current.captureScope();

  rerender({ scopeKey: "task-2:session-2" });

  let delivered = true;
  act(() => {
    delivered = result.current.deliver("original", GENERATED_RESULT, generation);
  });

  expect(delivered).toBe(false);
  expect(apply).not.toHaveBeenCalled();
  expect(result.current.pendingResult).toBeNull();
  expect(mockToast).not.toHaveBeenCalled();
});
