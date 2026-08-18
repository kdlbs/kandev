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
    WorkspaceAgentChat: (props: { sessionId: string }) => (
      <div data-testid="chat-loaded">{props.sessionId}</div>
    ),
  };
});

describe("host.ui.WorkspaceAgentChat", () => {
  // The chat graph (message list, composer, clarification overlay) must not
  // ride along on every plugin's boot, only on plugins that render it.
  it("is exposed without importing the chat module at boot", () => {
    const host = buildHostApi("coordinator", createAppStore());

    expect(host.ui.WorkspaceAgentChat).toBeTypeOf("function");
    expect(workspaceAgentChatModuleLoaded).not.toHaveBeenCalled();
  });

  // Lazily loading it is only acceptable if a plugin that renders it still
  // ends up with the real chat surface.
  it("resolves to the real chat surface when a plugin renders it", async () => {
    const host = buildHostApi("coordinator", createAppStore());
    const Chat = host.ui.WorkspaceAgentChat as React.ComponentType<{
      workspaceId: string;
      conversationKey: string;
      sessionId: string;
    }>;

    render(<Chat workspaceId="ws-1" conversationKey="coordinator" sessionId="session-42" />);

    expect(await screen.findByTestId("chat-loaded")).toBeTruthy();
    expect(workspaceAgentChatModuleLoaded).toHaveBeenCalled();
  });
});
