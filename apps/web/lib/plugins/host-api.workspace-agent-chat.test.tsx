import { describe, expect, it, vi } from "vitest";
import * as React from "react";
import { render, screen } from "@testing-library/react";
import { createAppStore } from "@/lib/state/store";
import { buildHostApi } from "./host-api";

const { workspaceAgentChatModuleLoaded } = vi.hoisted(() => ({
  workspaceAgentChatModuleLoaded: vi.fn(),
}));

vi.mock("@/components/agent-conversation/workspace-agent-chat", () => {
  workspaceAgentChatModuleLoaded();
  return {
    WorkspaceAgentChat: (props: { conversationId: string }) => (
      <div data-testid="chat-loaded">{props.conversationId}</div>
    ),
  };
});

describe("host.ui.WorkspaceAgentChat", () => {
  it("is exposed without importing the chat module at boot", () => {
    const host = buildHostApi("plugin-history", createAppStore());

    expect(host.ui.WorkspaceAgentChat).toBeTypeOf("function");
    expect(workspaceAgentChatModuleLoaded).not.toHaveBeenCalled();
  });

  it("resolves the managed conversation surface when a plugin renders it", async () => {
    const host = buildHostApi("plugin-history", createAppStore());
    const Chat = host.ui.WorkspaceAgentChat as React.ComponentType<{
      workspaceId: string;
      conversationId: string;
      resourceVersion: string;
    }>;

    render(<Chat workspaceId="ws-1" conversationId="session-42" resourceVersion="1" />);

    expect((await screen.findByTestId("chat-loaded")).textContent).toBe("session-42");
    expect(workspaceAgentChatModuleLoaded).toHaveBeenCalled();
  });
});
