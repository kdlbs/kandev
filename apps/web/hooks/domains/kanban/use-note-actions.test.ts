import { act, renderHook } from "@testing-library/react";
import { describe, expect, it, vi, beforeEach } from "vitest";

const mockWsRequest = vi.fn();
const mockToast = vi.fn();
const mockGetWebSocketClient = vi.fn();

vi.mock("@/components/toast-provider", () => ({
  useToast: () => ({ toast: mockToast }),
}));

vi.mock("@/lib/ws/connection", () => ({
  getWebSocketClient: () => mockGetWebSocketClient(),
}));

import { buildEnhanceNoteWithAIContent, useNoteActions } from "./use-note-actions";

describe("buildEnhanceNoteWithAIContent", () => {
  it("includes the note content and MCP instruction", () => {
    const prompt = buildEnhanceNoteWithAIContent("Current note");
    expect(prompt).toContain("update_task_note_kandev MCP tool");
    expect(prompt).toContain("Current note");
  });
});

describe("useNoteActions", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mockWsRequest.mockResolvedValue({ success: true });
    mockGetWebSocketClient.mockReturnValue({ request: mockWsRequest });
  });

  it("sends a message.add request for note enhancement", async () => {
    const { result } = renderHook(() =>
      useNoteActions({ resolvedSessionId: "session-1", taskId: "task-1" }),
    );

    let ok = false;
    await act(async () => {
      ok = await result.current.enhanceNoteWithAI("Improve me");
    });

    expect(ok).toBe(true);
    expect(mockWsRequest).toHaveBeenCalledWith(
      "message.add",
      expect.objectContaining({ task_id: "task-1", session_id: "session-1" }),
      10000,
    );
  });

  it("toasts and returns false when the WebSocket client is unavailable", async () => {
    mockGetWebSocketClient.mockReturnValue(null);
    const { result } = renderHook(() =>
      useNoteActions({ resolvedSessionId: "session-1", taskId: "task-1" }),
    );

    let ok = true;
    await act(async () => {
      ok = await result.current.enhanceNoteWithAI("Improve me");
    });

    expect(ok).toBe(false);
    expect(mockWsRequest).not.toHaveBeenCalled();
    expect(mockToast).toHaveBeenCalledWith(expect.objectContaining({ variant: "error" }));
  });
});
