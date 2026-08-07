import { act, renderHook } from "@testing-library/react";
import { useCallback, useRef, useState } from "react";
import { beforeEach, describe, expect, it, vi } from "vitest";

import type { TaskFormInputsHandle } from "@/components/task-create-dialog-types";
import type { UtilityGenerationResult } from "@/hooks/use-utility-agent-generator";

const mockToast = vi.fn();
const mockEnhancePrompt = vi.fn();

vi.mock("@/components/toast-provider", () => ({
  useToast: () => ({ toast: mockToast }),
}));

vi.mock("@/hooks/use-utility-agent-generator", () => ({
  useUtilityAgentGenerator: () => ({
    enhancePrompt: mockEnhancePrompt,
    isEnhancingPrompt: false,
  }),
}));

import { useSessionPromptController } from "./new-session-dialog";

const GENERATED_RESULT = {
  content: "improved prompt",
  callId: "call-123",
  durationMs: 1_200,
} satisfies UtilityGenerationResult;
const ORIGINAL_PROMPT = "original prompt";

function useSessionPromptHarness(initialPrompt = ORIGINAL_PROMPT, hasTarget = true) {
  const valueRef = useRef(initialPrompt);
  const [promptValue, setPromptValue] = useState(initialPrompt);
  const [hasPrompt, setHasPrompt] = useState(Boolean(initialPrompt.trim()));
  const updatePrompt = useCallback((value: string) => {
    valueRef.current = value;
    setPromptValue(value);
    setHasPrompt(value.trim().length > 0);
  }, []);
  const promptRef = useRef<TaskFormInputsHandle | null>(
    hasTarget
      ? {
          getValue: () => valueRef.current,
          setValue: updatePrompt,
          getAttachments: () => [],
        }
      : null,
  );
  const controller = useSessionPromptController(promptRef, "task-1");

  return {
    ...controller,
    promptRef,
    promptValue,
    setPromptValue: updatePrompt,
    hasPrompt,
  };
}

describe("useSessionPromptController", () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it("applies the enhanced prompt immediately when the source text is unchanged", async () => {
    mockEnhancePrompt.mockImplementation(
      async (_source: string, deliver: (result: UtilityGenerationResult) => Promise<boolean>) =>
        deliver(GENERATED_RESULT),
    );

    const { result } = renderHook(() => useSessionPromptHarness());

    await act(async () => {
      await result.current.handleEnhancePrompt();
    });

    expect(result.current.promptValue).toBe(GENERATED_RESULT.content);
    expect(result.current.hasPrompt).toBe(true);
    expect(result.current.pendingResult).toBeNull();
    expect(mockToast).toHaveBeenCalledWith(
      expect.objectContaining({ description: "Enhanced prompt applied.", variant: "success" }),
    );
  });

  it("retains the enhanced prompt when the user edits the text before delivery and applies it on demand", async () => {
    let deliverResult: ((result: UtilityGenerationResult) => Promise<boolean>) | undefined;
    mockEnhancePrompt.mockImplementation(
      async (_source: string, deliver: (result: UtilityGenerationResult) => Promise<boolean>) => {
        deliverResult = deliver;
      },
    );

    const { result } = renderHook(() => useSessionPromptHarness());

    await act(async () => {
      await result.current.handleEnhancePrompt();
    });

    act(() => {
      result.current.setPromptValue("edited prompt");
    });

    await act(async () => {
      await deliverResult?.(GENERATED_RESULT);
    });

    expect(result.current.promptValue).toBe("edited prompt");
    expect(result.current.pendingResult).toEqual(GENERATED_RESULT);

    act(() => {
      result.current.applyPending();
    });

    expect(result.current.promptValue).toBe(GENERATED_RESULT.content);
    expect(result.current.pendingResult).toBeNull();
  });

  it("preserves exact source text and retains the result after a whitespace-only edit", async () => {
    let deliverResult: ((result: UtilityGenerationResult) => Promise<boolean>) | undefined;
    mockEnhancePrompt.mockImplementation(
      async (_source: string, deliver: (result: UtilityGenerationResult) => Promise<boolean>) => {
        deliverResult = deliver;
      },
    );

    const initialPrompt = "  original prompt  ";
    const editedPrompt = "  original prompt   ";
    const { result } = renderHook(() => useSessionPromptHarness(initialPrompt));

    await act(async () => {
      await result.current.handleEnhancePrompt();
    });

    expect(mockEnhancePrompt).toHaveBeenCalledWith(initialPrompt, expect.any(Function));

    act(() => {
      result.current.setPromptValue(editedPrompt);
    });

    await act(async () => {
      await deliverResult?.(GENERATED_RESULT);
    });

    expect(result.current.promptValue).toBe(editedPrompt);
    expect(result.current.pendingResult).toEqual(GENERATED_RESULT);
  });

  it("retains the enhanced prompt when the target is unavailable", async () => {
    let deliverResult: ((result: UtilityGenerationResult) => Promise<boolean>) | undefined;
    mockEnhancePrompt.mockImplementation(
      async (_source: string, deliver: (result: UtilityGenerationResult) => Promise<boolean>) =>
        void (deliverResult = deliver),
    );

    const { result } = renderHook(() => useSessionPromptHarness(ORIGINAL_PROMPT));

    await act(async () => {
      await result.current.handleEnhancePrompt();
    });

    act(() => {
      result.current.promptRef.current = null;
    });

    await act(async () => {
      await deliverResult?.(GENERATED_RESULT);
    });

    expect(result.current.promptValue).toBe(ORIGINAL_PROMPT);
    expect(result.current.pendingResult).toEqual(GENERATED_RESULT);
    expect(mockToast).not.toHaveBeenCalledWith(
      expect.objectContaining({ description: "Enhanced prompt applied.", variant: "success" }),
    );
  });
});
