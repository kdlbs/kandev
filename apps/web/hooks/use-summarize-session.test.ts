import { act, renderHook } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { createElement, type ReactNode } from "react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { qk } from "@/lib/query/keys";
import { useSummarizeSession } from "./use-summarize-session";

const mockListMessages = vi.fn();
const mockExecuteUtilityPrompt = vi.fn();

vi.mock("@/lib/api/domains/session-api", () => ({
  listTaskSessionMessages: (...args: unknown[]) => mockListMessages(...args),
}));

vi.mock("@/lib/api/domains/utility-api", () => ({
  executeUtilityPrompt: (...args: unknown[]) => mockExecuteUtilityPrompt(...args),
}));

function renderSummarizeSession() {
  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  const wrapper = ({ children }: { children: ReactNode }) =>
    createElement(QueryClientProvider, { client: queryClient }, children);
  return { queryClient, ...renderHook(() => useSummarizeSession(), { wrapper }) };
}

describe("useSummarizeSession", () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it("returns the generated summary", async () => {
    mockListMessages.mockResolvedValue({
      messages: [{ type: "message", author_type: "user", content: "hello" }],
    });
    mockExecuteUtilityPrompt.mockResolvedValue({ success: true, response: "summary" });
    const { result } = renderSummarizeSession();

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
    const { result } = renderSummarizeSession();

    let summary;
    await act(async () => {
      summary = await result.current.summarize("session-1");
    });

    expect(summary).toEqual({ summary: null, error: "connection refused" });
    expect(result.current.isSummarizing).toBe(false);
  });

  it("fetches a fresh transcript even when the messages page cache is fresh", async () => {
    mockListMessages.mockResolvedValue({
      messages: [{ type: "message", author_type: "assistant", content: "fresh transcript" }],
    });
    mockExecuteUtilityPrompt.mockResolvedValue({ success: true, response: "summary" });
    const { queryClient, result } = renderSummarizeSession();
    queryClient.setQueryData(qk.session.messagesPage("session-1", { sort: "asc" }), {
      messages: [{ type: "message", author_type: "assistant", content: "cached transcript" }],
    });

    await act(async () => {
      await result.current.summarize("session-1");
    });

    expect(mockListMessages).toHaveBeenCalledTimes(1);
    expect(mockExecuteUtilityPrompt).toHaveBeenCalledWith(
      expect.objectContaining({ conversation_history: "Agent: fresh transcript" }),
    );
  });
});
