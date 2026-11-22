import { afterEach, describe, expect, it, vi } from "vitest";
import { getQuickChatCreation, startQuickChat } from "./workspace-api";

afterEach(() => vi.unstubAllGlobals());

describe("Quick Chat creation API", () => {
  it("sends the stable creation identity with a new request", async () => {
    const fetchMock = vi.fn().mockResolvedValue(
      new Response(
        JSON.stringify({
          task_id: "task-1",
          session_id: "session-1",
          creation_id: "create-1",
          creation_complete: true,
        }),
        { status: 200, headers: { "Content-Type": "application/json" } },
      ),
    );
    vi.stubGlobal("fetch", fetchMock);

    const response = await startQuickChat("workspace-1", {
      creation_id: "create-1",
      agent_profile_id: "agent-1",
      title: "Chat 1",
      repositories: [],
    });

    expect(fetchMock).toHaveBeenCalledWith(
      expect.stringContaining("/api/v1/workspaces/workspace-1/quick-chat"),
      expect.objectContaining({
        method: "POST",
        body: JSON.stringify({
          creation_id: "create-1",
          agent_profile_id: "agent-1",
          title: "Chat 1",
          repositories: [],
        }),
        credentials: "include",
      }),
    );
    expect(response).toMatchObject({ creation_id: "create-1", creation_complete: true });
  });

  it("looks up only the exact creation identity", async () => {
    const fetchMock = vi.fn().mockResolvedValue(
      new Response(
        JSON.stringify({
          task_id: "task-1",
          creation_id: "create-1",
          creation_complete: false,
        }),
        { status: 200, headers: { "Content-Type": "application/json" } },
      ),
    );
    vi.stubGlobal("fetch", fetchMock);

    await expect(getQuickChatCreation("workspace-1", "create-1")).resolves.toMatchObject({
      task_id: "task-1",
      creation_id: "create-1",
      creation_complete: false,
    });
    expect(fetchMock).toHaveBeenCalledWith(
      expect.stringContaining("/workspaces/workspace-1/quick-chat/creations/create-1"),
      expect.objectContaining({ credentials: "include" }),
    );
  });
});
