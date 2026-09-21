import { afterEach, expect, it, vi } from "vitest";
import {
  getConversationComments,
  getConversationCommentPage,
  postConversationComment,
  retryConversation,
} from "./orchestration-conversation-api";
afterEach(() => vi.unstubAllGlobals());
it("uses independent endpoints and preserves comment run state", async () => {
  const fetcher = vi.fn().mockResolvedValue(
    new Response(
      JSON.stringify({
        comments: [
          {
            id: "c",
            task_id: "t",
            author_id: "chief",
            author_type: "agent",
            body: "Finished",
            created_at: "2026-09-08",
            source: "session",
            run_id: "r",
            run_status: "finished",
          },
        ],
      }),
      { status: 200 },
    ),
  );
  vi.stubGlobal("fetch", fetcher);
  const rows = await getConversationComments("t");
  expect(rows[0]).toMatchObject({
    content: "Finished",
    authorId: "chief",
    runId: "r",
    runStatus: "finished",
  });
  expect(fetcher.mock.calls[0][0]).toContain("/api/v1/orchestration/tasks/t/comments");
  fetcher.mockImplementation(async () => new Response("{}", { status: 200 }));
  await postConversationComment("t", { body: "Hello" });
  await retryConversation("t", "session", "resume");
  expect(fetcher.mock.calls.map((call) => call[0])).toEqual(
    expect.arrayContaining([expect.stringContaining("/api/v1/orchestration/tasks/t/retry")]),
  );
  expect(fetcher.mock.calls.every((call) => !String(call[0]).includes("/office/"))).toBe(true);
});

it("shows an accepted user message as queued before a run exists", async () => {
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
              body: "Still waiting",
              created_at: "2026-09-18",
              receipt_status: "accepted",
            },
          ],
          next_cursor: "",
        }),
        { status: 200 },
      ),
    ),
  );

  const page = await getConversationCommentPage("t");

  expect(page.comments[0]).toMatchObject({
    receiptStatus: "accepted",
    runStatus: "queued",
  });
});
