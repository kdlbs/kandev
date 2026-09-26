import { afterEach, describe, expect, it, vi } from "vitest";
import {
  createConversationForkDraft,
  discardConversationForkDraft,
  estimateConversationForkDraft,
  getConversationForkContent,
  getConversationForkDraft,
  listConversationForkCandidates,
} from "./conversation-fork-api";

const options = { baseUrl: "http://backend.test" };

afterEach(() => vi.unstubAllGlobals());

describe("conversation fork API", () => {
  it("requests authorized cutoff and attachment candidates", async () => {
    const fetchMock = vi.fn().mockResolvedValue(
      new Response(JSON.stringify({ task_id: "task-1", session_id: "session-1" }), {
        status: 200,
      }),
    );
    vi.stubGlobal("fetch", fetchMock);

    await listConversationForkCandidates(
      "session /1",
      "message?1",
      { attachmentCursor: "next cursor", startMessageId: "message-0" },
      options,
    );

    expect(fetchMock.mock.calls[0]?.[0]).toBe(
      "http://backend.test/api/v1/task-sessions/session%20%2F1/fork-candidates?cutoff_message_id=message%3F1&start_message_id=message-0&attachment_cursor=next+cursor",
    );
  });

  it("creates a frozen draft with its selected range, evidence, model, and attachments", async () => {
    const fetchMock = vi
      .fn()
      .mockResolvedValue(
        new Response(JSON.stringify({ id: "fork-1", state: "draft" }), { status: 201 }),
      );
    vi.stubGlobal("fetch", fetchMock);

    await createConversationForkDraft(
      "session-1",
      {
        cutoff_message_id: "message-3",
        start_message_id: "message-2",
        draft_request_id: "draft-1",
        include_tool_evidence: true,
        model_id: "model-a",
        attachment_ids: ["attachment-1"],
      },
      options,
    );

    expect(fetchMock.mock.calls[0]?.[1]).toMatchObject({
      method: "POST",
      body: JSON.stringify({
        cutoff_message_id: "message-3",
        start_message_id: "message-2",
        draft_request_id: "draft-1",
        include_tool_evidence: true,
        model_id: "model-a",
        attachment_ids: ["attachment-1"],
      }),
    });
  });

  it("reads snapshot metadata and the matching compiled content", async () => {
    const fetchMock = vi
      .fn()
      .mockResolvedValueOnce(new Response(JSON.stringify({ id: "fork-1" }), { status: 200 }))
      .mockResolvedValueOnce(
        new Response(JSON.stringify({ content: "frozen history", content_hash: "hash-1" }), {
          status: 200,
        }),
      );
    vi.stubGlobal("fetch", fetchMock);

    await expect(getConversationForkDraft("fork/1", options)).resolves.toMatchObject({
      id: "fork-1",
    });
    await expect(getConversationForkContent("fork/1", options)).resolves.toMatchObject({
      content: "frozen history",
      content_hash: "hash-1",
    });
    expect(fetchMock.mock.calls.map(([url]) => url)).toEqual([
      "http://backend.test/api/v1/conversation-forks/fork%2F1",
      "http://backend.test/api/v1/conversation-forks/fork%2F1/content",
    ]);
  });

  it("estimates against the chosen target model and discards only unattached drafts", async () => {
    const fetchMock = vi
      .fn()
      .mockResolvedValueOnce(
        new Response(JSON.stringify({ estimated_tokens: 12 }), { status: 200 }),
      )
      .mockResolvedValueOnce(new Response(null, { status: 204 }));
    vi.stubGlobal("fetch", fetchMock);

    await estimateConversationForkDraft("fork-1", "model-b", options);
    await discardConversationForkDraft("fork-1", options);

    expect(fetchMock.mock.calls[0]?.[1]).toMatchObject({
      method: "POST",
      body: JSON.stringify({ model_id: "model-b" }),
    });
    expect(fetchMock.mock.calls[1]?.[1]).toMatchObject({ method: "DELETE" });
  });
});
