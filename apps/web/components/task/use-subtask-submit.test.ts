import { act, renderHook } from "@testing-library/react";
import { useState } from "react";
import { beforeEach, describe, expect, it, vi } from "vitest";

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

import { useSubtaskPromptZone } from "./use-subtask-submit";

const GENERATED_RESULT = {
  content: "improved prompt",
  callId: "call-123",
  durationMs: 1_200,
} satisfies UtilityGenerationResult;
const ORIGINAL_PROMPT = "original prompt";

function useSubtaskPromptHarness(initialPrompt = ORIGINAL_PROMPT) {
  const [promptValue, setPromptValue] = useState(initialPrompt);
  const [hasPrompt, setHasPrompt] = useState(Boolean(initialPrompt.trim()));
  const promptZone = useSubtaskPromptZone({
    taskTitle: "Parent task",
    inputDisabled: false,
    contextValue: "blank",
    initialPrompt: null,
    promptValue,
    setPromptValue,
    setHasPrompt,
  });

  return {
    ...promptZone,
    promptValue,
    setPromptValue,
    hasPrompt,
  };
}

describe("useSubtaskPromptZone", () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it("applies an enhanced prompt immediately when the source text is unchanged", async () => {
    mockEnhancePrompt.mockImplementation(
      async (_source: string, deliver: (result: UtilityGenerationResult) => Promise<boolean>) =>
        deliver(GENERATED_RESULT),
    );

    const { result } = renderHook(() => useSubtaskPromptHarness());

    act(() => {
      result.current.promptRef.current = { value: ORIGINAL_PROMPT } as HTMLTextAreaElement;
    });

    await act(async () => {
      await result.current.handleEnhancePrompt();
    });

    expect(result.current.promptValue).toBe("improved prompt");
    expect(result.current.hasPrompt).toBe(true);
    expect(result.current.pendingResult).toBeNull();
    expect(mockToast).toHaveBeenCalledWith(
      expect.objectContaining({ description: "Enhanced prompt applied.", variant: "success" }),
    );
  });

  it("retains the enhanced prompt when the user changes the source text before delivery", async () => {
    let deliverResult: ((result: UtilityGenerationResult) => Promise<boolean>) | undefined;
    mockEnhancePrompt.mockImplementation(
      async (_source: string, deliver: (result: UtilityGenerationResult) => Promise<boolean>) => {
        deliverResult = deliver;
      },
    );

    const { result } = renderHook(() => useSubtaskPromptHarness());

    act(() => {
      result.current.promptRef.current = { value: ORIGINAL_PROMPT } as HTMLTextAreaElement;
    });

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

    expect(result.current.promptValue).toBe("improved prompt");
    expect(result.current.pendingResult).toBeNull();
  });

  it("retains the enhanced prompt when the input target is unavailable", async () => {
    mockEnhancePrompt.mockImplementation(
      async (_source: string, deliver: (result: UtilityGenerationResult) => Promise<boolean>) =>
        deliver(GENERATED_RESULT),
    );

    const { result } = renderHook(() => useSubtaskPromptHarness());

    act(() => {
      result.current.promptRef.current = null;
    });

    await act(async () => {
      await result.current.handleEnhancePrompt();
    });

    expect(result.current.promptValue).toBe(ORIGINAL_PROMPT);
    expect(result.current.pendingResult).toEqual(GENERATED_RESULT);
  });

  it("resolves submission prompts from the same controlled prompt state", () => {
    const { result } = renderHook(() => useSubtaskPromptHarness());

    act(() => {
      result.current.setPromptValue("next prompt");
    });

    expect(result.current.resolvePrompt()).toBe("next prompt");
  });
});
