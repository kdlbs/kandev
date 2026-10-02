import { act, renderHook } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import type { QuickChatOpeningPayload } from "@/lib/state/slices/ui/types";

const startQuickChatMock = vi.hoisted(() => vi.fn());
const toastMock = vi.hoisted(() => vi.fn());

vi.mock("@/lib/api/domains/workspace-api", () => ({ startQuickChat: startQuickChatMock }));
vi.mock("@/components/toast-provider", () => ({ useToast: () => ({ toast: toastMock }) }));
vi.mock("@/lib/agent-profile-recent-use", () => ({
  recordAgentProfileRecentUseBestEffort: vi.fn(),
}));
vi.mock("@/lib/quick-chat/rename", () => ({
  persistQuickChatRename: () => Promise.resolve(),
}));

import { useAgentSelection } from "./use-quick-chat-modal";

const WORKSPACE_ID = "workspace-1";
const openingPayload: QuickChatOpeningPayload = {
  message: "Explain this file",
  clientMessageId: "opening-payload",
  attachments: [
    {
      type: "resource",
      mime_type: "text/plain",
      name: "notes.txt",
      size_bytes: 12,
      attachment_id: "attachment-1",
    },
  ],
};

function makeStore() {
  return {
    activeSessionId: "",
    sessions: [],
    agentProfiles: [
      { id: "passthrough", label: "Passthrough", cli_passthrough: true },
      { id: "structured", label: "Structured", cli_passthrough: false },
    ],
    agentGeneratedTaskTitles: false,
    closeQuickChatSession: vi.fn(),
    openQuickChat: vi.fn(),
    setQuickChatInitialPrompt: vi.fn(),
    renameQuickChatSession: vi.fn(),
    applyAgentProfileRecentUse: vi.fn(),
  } as unknown as Parameters<typeof useAgentSelection>[1];
}

beforeEach(() => {
  vi.clearAllMocks();
  startQuickChatMock
    .mockResolvedValueOnce({ task_id: "task-1", session_id: "session-1" })
    .mockResolvedValueOnce({ task_id: "task-2", session_id: "session-2" });
});

describe("Quick Chat opening payload routing", () => {
  it("delivers attachments over HTTP for passthrough and through session admission for structured", async () => {
    const store = makeStore();
    const { result } = renderHook(() => useAgentSelection(WORKSPACE_ID, store));

    await act(async () => {
      await result.current.handleSelectAgent("passthrough", [], openingPayload);
    });
    await act(async () => {
      await result.current.handleSelectAgent("structured", [], openingPayload);
    });

    expect(startQuickChatMock).toHaveBeenNthCalledWith(
      1,
      WORKSPACE_ID,
      expect.objectContaining({
        prompt: openingPayload.message,
        attachments: openingPayload.attachments,
      }),
    );
    expect(startQuickChatMock).toHaveBeenNthCalledWith(
      2,
      WORKSPACE_ID,
      expect.not.objectContaining({ prompt: openingPayload.message }),
    );
    expect(store.setQuickChatInitialPrompt).toHaveBeenCalledWith("session-2", openingPayload);
  });
});
