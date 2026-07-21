import { fireEvent, render, screen } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";

import type { UtilityGenerationResult } from "@/hooks/use-utility-agent-generator";

import { PromptResultRecovery } from "./prompt-result-recovery";

const GENERATED_RESULT = {
  content: "enhanced prompt output",
  callId: "call-123",
  durationMs: 1_200,
} satisfies UtilityGenerationResult;

describe("PromptResultRecovery", () => {
  beforeEach(() => {
    Object.defineProperty(navigator, "clipboard", {
      configurable: true,
      value: { writeText: vi.fn().mockResolvedValue(undefined) },
    });
  });

  it("offers apply and copy actions without exposing the raw enhanced prompt", async () => {
    const onApply = vi.fn();
    const onCopy = vi.fn(async () => {
      await navigator.clipboard.writeText(GENERATED_RESULT.content);
    });

    render(
      <PromptResultRecovery pendingResult={GENERATED_RESULT} onApply={onApply} onCopy={onCopy} />,
    );

    const recovery = screen.getByTestId("prompt-result-recovery");
    expect(recovery.getAttribute("aria-live")).toBe("polite");
    expect(recovery.textContent).toContain("An enhanced prompt is available.");
    expect(recovery.textContent).not.toContain(GENERATED_RESULT.content);
    expect(recovery.outerHTML).not.toContain(GENERATED_RESULT.content);

    fireEvent.click(screen.getByRole("button", { name: "Apply" }));
    expect(onApply).toHaveBeenCalledTimes(1);

    fireEvent.click(screen.getByRole("button", { name: "Copy" }));
    expect(onCopy).toHaveBeenCalledTimes(1);
    expect(navigator.clipboard.writeText).toHaveBeenCalledWith(GENERATED_RESULT.content);
  });
});
