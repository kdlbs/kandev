import { afterEach, expect, it, vi } from "vitest";
import {
  getAssistant,
  getAssistantPage,
  resolveAssistantInput,
  controlAssistant,
} from "./assistant-api";
import {
  createConversationSender,
  getConversationCommentPage,
} from "./orchestration-conversation-api";
afterEach(() => vi.unstubAllGlobals());
it("keeps list continuations and forwards exact native operation identity", async () => {
  const fetcher = vi
    .fn()
    .mockImplementation(
      async () =>
        new Response(JSON.stringify({ entries: [], next_cursor: "next" }), { status: 200 }),
    );
  vi.stubGlobal("fetch", fetcher);
  expect((await getAssistantPage("attention", "scope cursor")).next_cursor).toBe("next");
  const operation = {
    operation_id: "stable",
    expected_intent_revision: 4,
    expected_binding_version: 2,
  };
  await resolveAssistantInput("attention/id", {
    ...operation,
    expected_revision: 3,
    source_revision: "generation",
    session_id: "original-session",
    option_id: "deny",
  });
  await controlAssistant({ ...operation, action: "pause" });
  expect(fetcher.mock.calls[0][0]).toContain("/assistant/attention?after=scope+cursor&limit=50");
  expect(fetcher.mock.calls[1][0]).toContain("/assistant/attention/attention%2Fid/resolve");
  expect(JSON.parse(fetcher.mock.calls[1][1].body)).toMatchObject({
    operation_id: "stable",
    session_id: "original-session",
    source_revision: "generation",
    option_id: "deny",
  });
  expect(JSON.parse(fetcher.mock.calls[2][1].body)).toEqual({ ...operation, action: "pause" });
});
it("distinguishes an unconfigured binding from unavailable service", async () => {
  const fetcher = vi.fn().mockImplementation(async () => new Response("", { status: 404 }));
  vi.stubGlobal("fetch", fetcher);
  expect(await getAssistant()).toBeNull();
  fetcher.mockImplementation(async () => new Response("", { status: 503 }));
  await expect(getAssistant()).rejects.toMatchObject({ status: 503 });
});
it("retries an uncertain send with its original id, then gives a new message a new id", async () => {
  const fetcher = vi
    .fn()
    .mockRejectedValueOnce(new Error("lost acknowledgement"))
    .mockImplementation(async () => new Response("{}", { status: 201 }));
  vi.stubGlobal("fetch", fetcher);
  const conversation = "private-conversation";
  const send = createConversationSender(conversation);
  await expect(
    send(conversation, { body: "Summarize the sample tasks", author_type: "user" }),
  ).rejects.toThrow();
  await send(conversation, { body: "Summarize the sample tasks", author_type: "user" });
  await send(conversation, { body: "Summarize the sample tasks", author_type: "user" });
  const ids = fetcher.mock.calls.map((call) => JSON.parse(call[1].body).client_message_id);
  expect(ids[0]).toBeTruthy();
  expect(ids[1]).toBe(ids[0]);
  expect(ids[2]).not.toBe(ids[0]);
  await expect(send("foreign", { body: "Example", author_type: "user" })).rejects.toThrow();
  expect(fetcher).toHaveBeenCalledTimes(3);
});
it("retains history cursors and delivery receipts without claiming completion", async () => {
  vi.stubGlobal(
    "fetch",
    vi.fn().mockResolvedValue(
      new Response(
        JSON.stringify({
          comments: [
            {
              id: "c",
              task_id: "t",
              author_id: "owner",
              author_type: "user",
              body: "Sample",
              created_at: "2026-09-18",
              client_message_id: "client",
              receipt_status: "accepted",
              intent_revision: 3,
              sequence: 3,
              run_status: "queued",
            },
          ],
          next_cursor: "older",
        }),
        { status: 200 },
      ),
    ),
  );
  const page = await getConversationCommentPage("t", "previous");
  expect(page.next_cursor).toBe("older");
  expect(page.comments[0]).toMatchObject({
    clientMessageId: "client",
    receiptStatus: "accepted",
    intentRevision: 3,
    runStatus: "queued",
  });
});
