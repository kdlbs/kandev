/* eslint-disable max-lines-per-function, sonarjs/no-duplicate-string -- This fixture exercises one managed conversation lifecycle. */
import * as React from "react";
import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { WorkspaceAgentChat } from "./workspace-agent-chat";

const transport = vi.hoisted(() => ({
  fetch: vi.fn(),
  messages: [{ id: "m-1", content: "existing transcript" }],
}));

vi.mock("@/lib/plugins/conversation-scope", () => ({
  pluginConversationUrl: (pluginId: string, path: string) => `/api/plugins/${pluginId}${path}`,
  fetchBinding: () => Promise.resolve({ bindingToken: "binding-token" }),
  ConversationScopeContext: React.createContext({
    ready: () => Promise.resolve({ bindingToken: "binding-token" }),
  }),
  PluginConversationScopeProvider: ({ children }: { children: React.ReactNode }) => children,
}));
vi.mock("@/lib/plugins/conversation-host", () => ({
  pluginConversationApi: {
    useSessionMessages: () => ({ messages: transport.messages, loading: false, removed: false }),
  },
}));

describe("WorkspaceAgentChat", () => {
  afterEach(cleanup);
  beforeEach(() => {
    transport.fetch.mockReset();
    vi.stubGlobal("fetch", transport.fetch);
  });

  it("loads an exact descriptor, renders its transcript, and dispatches through the scoped bridge", async () => {
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
      .mockResolvedValueOnce(new Response(JSON.stringify({ status: "sent" }), { status: 200 }));
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
    await waitFor(() => expect(transport.fetch).toHaveBeenCalledTimes(2));
    expect(transport.fetch.mock.calls[0][1].headers).toEqual({
      "X-Kandev-Plugin-Binding": "binding-token",
    });
    expect(transport.fetch.mock.calls[1][1].headers).toMatchObject({
      "X-Kandev-Plugin-Binding": "binding-token",
    });
    expect(transport.fetch.mock.calls[1][0]).toContain(
      "/managed/session-1/dispatch?workspace_id=ws-1",
    );
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
      .mockResolvedValueOnce(new Response(null, { status: 503 }));
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
      .mockResolvedValueOnce(
        new Response(JSON.stringify({ status: "skipped_busy" }), { status: 200 }),
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
});
