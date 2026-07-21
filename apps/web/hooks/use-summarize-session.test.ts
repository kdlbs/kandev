import { act, renderHook } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { useSummarizeSession } from "./use-summarize-session";

const mockListMessages = vi.fn();
const mockExecuteUtilityPrompt = vi.fn();

vi.mock("@/lib/api/domains/session-api", () => ({
  listTaskSessionMessages: (...args: unknown[]) => mockListMessages(...args),
}));

vi.mock("@/lib/api/domains/utility-api", () => ({
  executeUtilityPrompt: (...args: unknown[]) => mockExecuteUtilityPrompt(...args),
}));

describe("useSummarizeSession", () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it("returns the generated summary", async () => {
    mockListMessages.mockResolvedValue({
      messages: [{ type: "message", author_type: "user", content: "hello" }],
    });
    mockExecuteUtilityPrompt.mockResolvedValue({ success: true, response: "summary" });
    const { result } = renderHook(() => useSummarizeSession());

    let summary;
    await act(async () => {
      summary = await result.current.summarize("session-1");
    });

    expect(summary).toEqual({ summary: "summary" });
  });

  it("returns backend execution errors instead of swallowing them", async () => {
    mockListMessages.mockResolvedValue({
      messages: [{ type: "message", author_type: "user", content: "hello" }],
    });
    mockExecuteUtilityPrompt.mockRejectedValue(new Error("connection refused"));
    const { result } = renderHook(() => useSummarizeSession());

    let summary;
    await act(async () => {
      summary = await result.current.summarize("session-1");
    });

    expect(summary).toEqual({ summary: null, error: "connection refused" });
    expect(result.current.isSummarizing).toBe(false);
  });
});
