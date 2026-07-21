import { act, fireEvent, render, renderHook, screen } from "@testing-library/react";
import { useRef, useState } from "react";
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

vi.mock("@/components/enhance-prompt-button", () => ({
  EnhancePromptButton: ({ onClick }: { onClick: () => void }) => (
    <button type="button" onClick={onClick}>
      Enhance
    </button>
  ),
}));

vi.mock("./session-dialog-shared", () => ({
  AttachButton: ({ onClick }: { onClick: () => void }) => (
    <button type="button" onClick={onClick}>
      Attach
    </button>
  ),
}));

import { SessionPromptField } from "./new-session-form-prompt";
import { useSessionPromptController } from "./new-session-dialog";

const GENERATED_RESULT = {
  content: "improved prompt",
  callId: "call-123",
  durationMs: 1_200,
} satisfies UtilityGenerationResult;

function useSessionPromptHarness(initialPrompt = "original prompt") {
  const promptRef = useRef<HTMLTextAreaElement | null>({ value: initialPrompt } as HTMLTextAreaElement);
  const [promptValue, setPromptValue] = useState(initialPrompt);
  const [hasPrompt, setHasPrompt] = useState(Boolean(initialPrompt.trim()));
  const controller = useSessionPromptController(promptRef, promptValue, setPromptValue, setHasPrompt);

  return {
    ...controller,
    promptRef,
    promptValue,
    setPromptValue,
    hasPrompt,
  };
}

describe("SessionPromptField", () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it("shows inline recovery controls only when an enhanced prompt is pending", async () => {
    const onApplyPending = vi.fn();
    const onCopyPending = vi.fn();

    const { rerender } = render(
      <SessionPromptField
        promptRef={{ current: null }}
        promptValue="original prompt"
        contextItems={[]}
        isBusy={false}
        isDragging={false}
        isSummarizing={false}
        hasPrompt={true}
        hasProfiles={true}
        isUtilityConfigured={true}
        isEnhancingPrompt={false}
        pendingResult={null}
        fileInputRef={{ current: null }}
        onPromptChange={vi.fn()}
        onPaste={vi.fn()}
        onSubmit={vi.fn()}
        onAttachClick={vi.fn()}
        onEnhancePrompt={vi.fn()}
        onApplyPending={onApplyPending}
        onCopyPending={onCopyPending}
        onDragOver={vi.fn()}
        onDragLeave={vi.fn()}
        onDrop={vi.fn()}
        onFileInputChange={vi.fn()}
      />,
    );

    expect(screen.queryByTestId("prompt-result-recovery")).toBeNull();

    rerender(
      <SessionPromptField
        promptRef={{ current: null }}
        promptValue="original prompt"
        contextItems={[]}
        isBusy={false}
        isDragging={false}
        isSummarizing={false}
        hasPrompt={true}
        hasProfiles={true}
        isUtilityConfigured={true}
        isEnhancingPrompt={false}
        pendingResult={GENERATED_RESULT}
        fileInputRef={{ current: null }}
        onPromptChange={vi.fn()}
        onPaste={vi.fn()}
        onSubmit={vi.fn()}
        onAttachClick={vi.fn()}
        onEnhancePrompt={vi.fn()}
        onApplyPending={onApplyPending}
        onCopyPending={onCopyPending}
        onDragOver={vi.fn()}
        onDragLeave={vi.fn()}
        onDrop={vi.fn()}
        onFileInputChange={vi.fn()}
      />,
    );

    expect(screen.getByTestId("prompt-result-recovery")).not.toBeNull();

    fireEvent.click(screen.getByRole("button", { name: "Apply" }));
    fireEvent.click(screen.getByRole("button", { name: "Copy" }));

    expect(onApplyPending).toHaveBeenCalledTimes(1);
    expect(onCopyPending).toHaveBeenCalledTimes(1);
  });
});

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

    expect(result.current.promptValue).toBe("improved prompt");
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

    expect(result.current.promptValue).toBe("improved prompt");
    expect(result.current.pendingResult).toBeNull();
  });
});
