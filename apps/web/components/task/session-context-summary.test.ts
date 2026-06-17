import { describe, expect, it, vi } from "vitest";
import { applySummarizeSessionResult } from "./session-context-summary";

describe("applySummarizeSessionResult", () => {
  it("sanitizes unsafe characters before writing the prompt", () => {
    const promptRef = { current: { value: "" } as HTMLTextAreaElement };
    const setContextValue = vi.fn();
    const setHasPrompt = vi.fn();
    const toast = vi.fn();

    applySummarizeSessionResult({
      result: { summary: "line1\r\n<unsafe>\nline2" },
      promptRef,
      setContextValue,
      setHasPrompt,
      toast,
    });

    expect(promptRef.current.value).toBe("line1\n unsafe \nline2");
    expect(setHasPrompt).toHaveBeenCalledWith(true);
    expect(toast).not.toHaveBeenCalled();
  });

  it("treats an empty summary string as a successful summary", () => {
    const promptRef = { current: { value: "existing" } as HTMLTextAreaElement };
    const setContextValue = vi.fn();
    const setHasPrompt = vi.fn();
    const toast = vi.fn();

    applySummarizeSessionResult({
      result: { summary: "" },
      promptRef,
      setContextValue,
      setHasPrompt,
      toast,
    });

    expect(promptRef.current.value).toBe("");
    expect(setHasPrompt).toHaveBeenCalledWith(true);
    expect(toast).not.toHaveBeenCalled();
  });

  it("resets context and shows an error toast when summary is null", () => {
    const promptRef = { current: { value: "" } as HTMLTextAreaElement };
    const setContextValue = vi.fn();
    const setHasPrompt = vi.fn();
    const toast = vi.fn();

    applySummarizeSessionResult({
      result: { summary: null, error: "connection refused" },
      promptRef,
      setContextValue,
      setHasPrompt,
      toast,
    });

    expect(setContextValue).toHaveBeenCalledWith("blank");
    expect(setHasPrompt).not.toHaveBeenCalled();
    expect(toast).toHaveBeenCalledWith({
      title: "Summarize failed",
      description: "connection refused",
      variant: "error",
    });
  });
});
