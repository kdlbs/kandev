import { renderHook } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import type { Message } from "@/lib/types/http";
import type { RenderItem } from "@/hooks/use-processed-messages";
import { useCopilotActivity } from "./use-copilot-activity";

const call = {
  id: "call",
  session_id: "s1",
  task_id: "t1",
  turn_id: "t",
  author_type: "agent",
  type: "tool_call",
  content: "",
  created_at: "2026-09-29T10:00:00Z",
  metadata: { status: "pending", tool_call_id: "c1" },
} as unknown as Message;
const request = {
  ...call,
  id: "req",
  type: "permission_request",
  metadata: { tool_call_id: "c1" },
} as unknown as Message;
const other = {
  ...call,
  id: "other",
  metadata: { status: "running", tool_call_id: "c2" },
} as Message;

describe("useCopilotActivity", () => {
  // The visible list has already merged the permission request into its tool
  // row and dropped it, so the awaiting rule must read the full message list.
  it("keeps a call awaiting permission visible, reading requests from the full list", () => {
    const groupedItems: RenderItem[] = [call, other].map((message) => ({
      type: "message",
      message,
    }));
    const { result } = renderHook(() =>
      useCopilotActivity(true, {
        groupedItems,
        messages: [call, request, other],
        activeTurnId: "t",
        session: { state: "WAITING_FOR_INPUT" },
      }),
    );
    expect(result.current.items).toEqual([{ type: "message", message: call }]);
    expect(result.current.statusLine?.verbKey).toBe("coordinator:activityVerbWaiting");
  });

  it("returns the default grouping untouched when off", () => {
    const groupedItems: RenderItem[] = [{ type: "message", message: call }];
    const { result } = renderHook(() =>
      useCopilotActivity(false, {
        groupedItems,
        messages: [call],
        activeTurnId: "t",
        session: { state: "RUNNING" },
      }),
    );
    expect(result.current.items).toBe(groupedItems);
    expect(result.current.statusLine).toBeNull();
  });
});
