import { fireEvent, render, screen } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";

import type { UtilityGenerationResult } from "@/hooks/use-utility-agent-generator";

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

import { PromptZone } from "./new-subtask-form-parts";

const GENERATED_RESULT = {
  content: "improved prompt",
  callId: "call-123",
  durationMs: 1_200,
} satisfies UtilityGenerationResult;

describe("PromptZone", () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it("shows inline recovery controls for retained enhanced prompts", async () => {
    const onApplyPending = vi.fn();
    const onCopyPending = vi.fn();

    render(
      <PromptZone
        promptRef={{ current: null }}
        promptValue="original prompt"
        contextItems={[]}
        attachments={{
          attachments: [],
          isDragging: false,
          fileInputRef: { current: null },
          handleRemoveAttachment: vi.fn(),
          handlePaste: vi.fn(),
          handleDragOver: vi.fn(),
          handleDragLeave: vi.fn(),
          handleDrop: vi.fn(),
          handleAttachClick: vi.fn(),
          handleFileInputChange: vi.fn(),
        }}
        isCreating={false}
        isSummarizing={false}
        isEnhancingPrompt={false}
        isUtilityConfigured={true}
        handleEnhancePrompt={vi.fn()}
        pendingResult={GENERATED_RESULT}
        onPromptChange={vi.fn()}
        onApplyPending={onApplyPending}
        onCopyPending={onCopyPending}
        onSubmitShortcut={vi.fn()}
      />,
    );

    expect(screen.getByTestId("prompt-result-recovery")).not.toBeNull();

    fireEvent.click(screen.getByRole("button", { name: "Apply" }));
    fireEvent.click(screen.getByRole("button", { name: "Copy" }));

    expect(onApplyPending).toHaveBeenCalledTimes(1);
    expect(onCopyPending).toHaveBeenCalledTimes(1);
  });
});
