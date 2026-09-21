/* eslint-disable max-lines-per-function, sonarjs/no-duplicate-string -- This fixture exercises one managed conversation lifecycle. */
import * as React from "react";
import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { WorkspaceAgentChat } from "./workspace-agent-chat";

const transport = vi.hoisted(() => ({
  fetch: vi.fn(),
  dispatch: vi.fn(),
  messages: [{ id: "m-1", content: "existing transcript" }],
  removed: false,
  transcriptError: null as { code: string } | null,
}));

vi.mock("@/lib/plugins/conversation-scope", () => ({
  pluginConversationUrl: (pluginId: string, path: string) => `/api/plugins/${pluginId}${path}`,
  fetchConversationBinding: () => Promise.resolve({ bindingToken: "binding-token" }),
  dispatchManagedConversation: transport.dispatch,
  ConversationScopeContext: React.createContext({
    ready: () => Promise.resolve({ bindingToken: "binding-token" }),
  }),
  PluginConversationScopeProvider: ({ children }: { children: React.ReactNode }) => children,
}));
vi.mock("@/lib/plugins/conversation-host", () => ({
  pluginConversationApi: {
    useSessionMessages: () => ({
      messages: transport.messages,
      loading: false,
      removed: transport.removed,
      error: transport.transcriptError,
    }),
  },
}));

describe("WorkspaceAgentChat", () => {
  afterEach(cleanup);
  beforeEach(() => {
    transport.fetch.mockReset();
    transport.dispatch.mockReset();
    transport.dispatch.mockResolvedValue(undefined);
    transport.transcriptError = null;
    transport.removed = false;
    vi.stubGlobal("fetch", transport.fetch);
  });

  it("loads an exact descriptor, renders its transcript, and dispatches through the scoped bridge", async () => {
    transport.fetch.mockResolvedValueOnce(
      new Response(
        JSON.stringify({
          taskId: "task-1",
          sessionId: "session-1",
          workspaceId: "ws-1",
          managedConversationToken: "managed-token",
        }),
        { status: 200 },
      ),
    );
    render(
      <WorkspaceAgentChat
        pluginId="plugin-1"
        workspaceId="ws-1"
        conversationId="session-1"
        resourceVersion="1"
      />,
    );
    await screen.findByTestId("workspace-agent-chat");
    expect(screen.getByText("existing transcript")).toBeTruthy();
    fireEvent.change(screen.getByLabelText("Message"), { target: { value: "hello" } });
    fireEvent.click(screen.getByRole("button", { name: "Send" }));
    await waitFor(() => expect(transport.dispatch).toHaveBeenCalledTimes(1));
    expect(transport.fetch.mock.calls[0][1].headers).toEqual({
      "X-Kandev-Plugin-Binding": "binding-token",
    });
    expect(transport.dispatch).toHaveBeenCalledWith({
      bindingToken: "binding-token",
      content: "hello",
      occurrenceKey: expect.any(String),
      pluginId: "plugin-1",
      sessionId: "session-1",
      workspaceId: "ws-1",
    });
  });

  it("clears the prompt when dispatch starts a new managed session", async () => {
    transport.fetch
      .mockResolvedValueOnce(
        new Response(
          JSON.stringify({
            taskId: "task-1",
            sessionId: "session-1",
            workspaceId: "ws-1",
            managedConversationToken: "managed-token",
          }),
          { status: 200 },
        ),
      )
      .mockResolvedValueOnce(new Response(JSON.stringify({ status: "started" }), { status: 200 }));
    render(
      <WorkspaceAgentChat
        pluginId="plugin-1"
        workspaceId="ws-1"
        conversationId="session-1"
        resourceVersion="1"
      />,
    );
    await screen.findByTestId("workspace-agent-chat");
    const composer = screen.getByLabelText("Message") as HTMLTextAreaElement;
    fireEvent.change(composer, { target: { value: "start the conversation" } });
    fireEvent.click(screen.getByRole("button", { name: "Send" }));
    await waitFor(() => expect(composer.value).toBe(""));
    expect(screen.queryByRole("alert")).toBeNull();
  });

  it("fails closed for a foreign descriptor and replaces the old scope when the resource version changes", async () => {
    transport.fetch
      .mockResolvedValueOnce(
        new Response(
          JSON.stringify({ taskId: "foreign", sessionId: "session-1", workspaceId: "ws-2" }),
          { status: 200 },
        ),
      )
      .mockResolvedValueOnce(
        new Response(
          JSON.stringify({
            taskId: "task-2",
            sessionId: "session-2",
            workspaceId: "ws-1",
            managedConversationToken: "managed-token",
          }),
          { status: 200 },
        ),
      );
    const view = render(
      <WorkspaceAgentChat
        pluginId="plugin-1"
        workspaceId="ws-1"
        conversationId="session-1"
        resourceVersion="1"
      />,
    );
    await waitFor(() =>
      expect(screen.getByTestId("workspace-agent-chat-status").dataset.status).toBe(
        "permission-denied",
      ),
    );
    view.rerender(
      <WorkspaceAgentChat
        pluginId="plugin-1"
        workspaceId="ws-1"
        conversationId="session-2"
        resourceVersion="2"
      />,
    );
    await screen.findByTestId("workspace-agent-chat");
    expect(transport.fetch.mock.calls[1][0]).toContain("/managed/session-2?workspace_id=ws-1");
  });

  it("keeps the prompt and announces a failed dispatch", async () => {
    transport.fetch.mockResolvedValueOnce(
      new Response(
        JSON.stringify({
          taskId: "task-1",
          sessionId: "session-1",
          workspaceId: "ws-1",
          managedConversationToken: "managed-token",
        }),
        { status: 200 },
      ),
    );
    transport.dispatch.mockRejectedValueOnce(new Error("dispatch unavailable"));
    render(
      <WorkspaceAgentChat
        pluginId="plugin-1"
        workspaceId="ws-1"
        conversationId="session-1"
        resourceVersion="1"
      />,
    );
    await screen.findByTestId("workspace-agent-chat");
    fireEvent.change(screen.getByLabelText("Message"), { target: { value: "retry me" } });
    fireEvent.click(screen.getByRole("button", { name: "Send" }));
    expect((await screen.findByRole("alert")).textContent).toBe(
      "Message could not be sent. Try again.",
    );
    expect((screen.getByLabelText("Message") as HTMLTextAreaElement).value).toBe("retry me");
  });

  it("keeps the prompt when a busy managed session does not accept the dispatch", async () => {
    transport.fetch.mockResolvedValueOnce(
      new Response(
        JSON.stringify({
          taskId: "task-1",
          sessionId: "session-1",
          workspaceId: "ws-1",
          managedConversationToken: "managed-token",
        }),
        { status: 200 },
      ),
    );
    transport.dispatch.mockRejectedValueOnce(new Error("dispatch rejected"));
    render(
      <WorkspaceAgentChat
        pluginId="plugin-1"
        workspaceId="ws-1"
        conversationId="session-1"
        resourceVersion="1"
      />,
    );
    await screen.findByTestId("workspace-agent-chat");
    fireEvent.change(screen.getByLabelText("Message"), { target: { value: "wait for turn" } });
    fireEvent.click(screen.getByRole("button", { name: "Send" }));
    await screen.findByRole("alert");
    expect((screen.getByLabelText("Message") as HTMLTextAreaElement).value).toBe("wait for turn");
  });

  it("renders a terminal status instead of a loading spinner for a deleted conversation", async () => {
    transport.fetch.mockResolvedValueOnce(new Response(null, { status: 404 }));
    render(
      <WorkspaceAgentChat
        pluginId="plugin-1"
        workspaceId="ws-1"
        conversationId="session-1"
        resourceVersion="1"
      />,
    );
    const terminal = await screen.findByTestId("workspace-agent-chat-status");
    await waitFor(() => expect(terminal.dataset.status).toBe("deleted"));
    expect(terminal.querySelector('[role="status"]')?.textContent).toBe("Conversation ended");
    expect(screen.queryByLabelText("Loading conversation…")).toBeNull();
  });

  it("surfaces a scoped transcript authorization failure", async () => {
    transport.transcriptError = { code: "unauthenticated" };
    transport.fetch.mockResolvedValueOnce(
      new Response(
        JSON.stringify({
          taskId: "task-1",
          sessionId: "session-1",
          workspaceId: "ws-1",
          managedConversationToken: "managed-token",
        }),
        { status: 200 },
      ),
    );
    render(
      <WorkspaceAgentChat
        pluginId="plugin-1"
        workspaceId="ws-1"
        conversationId="session-1"
        resourceVersion="1"
      />,
    );
    await waitFor(() =>
      expect(screen.getByTestId("workspace-agent-chat-status").dataset.status).toBe(
        "permission-denied",
      ),
    );
  });

  it("reports deleted when a live managed transcript removal arrives", async () => {
    const onStatus = vi.fn();
    transport.fetch.mockResolvedValueOnce(
      new Response(
        JSON.stringify({
          taskId: "task-1",
          sessionId: "session-1",
          workspaceId: "ws-1",
          managedConversationToken: "managed-token",
        }),
        { status: 200 },
      ),
    );
    const view = render(
      <WorkspaceAgentChat
        pluginId="plugin-1"
        workspaceId="ws-1"
        conversationId="session-1"
        resourceVersion="1"
        onStatus={onStatus}
      />,
    );
    await screen.findByTestId("workspace-agent-chat");
    transport.removed = true;
    transport.fetch.mockResolvedValueOnce(
      new Response(
        JSON.stringify({
          taskId: "task-1",
          sessionId: "session-1",
          workspaceId: "ws-1",
          managedConversationToken: "managed-token",
        }),
        { status: 200 },
      ),
    );
    view.rerender(
      <WorkspaceAgentChat
        pluginId="plugin-1"
        workspaceId="ws-1"
        conversationId="session-1"
        resourceVersion="2"
        onStatus={onStatus}
      />,
    );
    await waitFor(() => expect(onStatus).toHaveBeenLastCalledWith("deleted"));
  });

  it("recovers from a terminal transcript failure when the managed conversation changes", async () => {
    transport.transcriptError = { code: "unauthenticated" };
    transport.fetch
      .mockResolvedValueOnce(
        new Response(
          JSON.stringify({
            taskId: "task-1",
            sessionId: "session-1",
            workspaceId: "ws-1",
            managedConversationToken: "managed-token-1",
          }),
          { status: 200 },
        ),
      )
      .mockResolvedValueOnce(
        new Response(
          JSON.stringify({
            taskId: "task-2",
            sessionId: "session-2",
            workspaceId: "ws-1",
            managedConversationToken: "managed-token-2",
          }),
          { status: 200 },
        ),
      );
    const view = render(
      <WorkspaceAgentChat
        pluginId="plugin-1"
        workspaceId="ws-1"
        conversationId="session-1"
        resourceVersion="1"
      />,
    );
    await waitFor(() =>
      expect(screen.getByTestId("workspace-agent-chat-status").dataset.status).toBe(
        "permission-denied",
      ),
    );

    transport.transcriptError = null;
    view.rerender(
      <WorkspaceAgentChat
        pluginId="plugin-1"
        workspaceId="ws-1"
        conversationId="session-2"
        resourceVersion="2"
      />,
    );

    await screen.findByTestId("workspace-agent-chat");
  });
});
