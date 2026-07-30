import { act, renderHook } from "@testing-library/react";
import { describe, expect, it, vi, beforeEach } from "vitest";

const mockWsRequest = vi.fn();
const mockToast = vi.fn();

vi.mock("@/components/toast-provider", () => ({
  useToast: () => ({ toast: mockToast }),
}));

vi.mock("@/lib/ws/connection", () => ({
  getWebSocketClient: () => ({ request: mockWsRequest }),
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
});
