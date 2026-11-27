import { afterEach, expect, it, vi } from "vitest";
import { searchTaskConversations } from "./conversation-search-api";

afterEach(() => vi.unstubAllGlobals());

it("encodes literal text and returns the conversation response", async () => {
  const response = { tasks: [], state: "ready", has_more: false };
  const fetchSpy = vi.fn(async (_url: RequestInfo | URL) => new Response(JSON.stringify(response)));
  vi.stubGlobal("fetch", fetchSpy);
  const query = 'café 100%_\\"中文测试';
  expect(await searchTaskConversations("ws-1", query, 5)).toEqual(response);
  expect(fetchSpy).toHaveBeenCalledTimes(1);
  const url = new URL(String(fetchSpy.mock.calls[0][0]));
  expect(url.pathname).toBe("/api/v1/workspaces/ws-1/tasks/conversations");
  expect(url.searchParams.get("query")).toBe(query);
  expect(url.searchParams.get("limit")).toBe("5");
  expect([...url.searchParams.keys()]).toEqual(["query", "limit"]);
});

it("propagates cancellation through the real fetchJson client", async () => {
  const controller = new AbortController();
  const fetchSpy = vi.fn(
    (_url: RequestInfo | URL, init?: RequestInit) =>
      new Promise<Response>((_resolve, reject) => {
        init?.signal?.addEventListener("abort", () => reject(init?.signal?.reason), { once: true });
      }),
  );
  vi.stubGlobal("fetch", fetchSpy);
  const request = searchTaskConversations("ws-1", "observatory", 20, {
    init: { signal: controller.signal },
  });
  expect(fetchSpy).toHaveBeenCalledWith(
    expect.any(String),
    expect.objectContaining({ signal: controller.signal }),
  );
  const rejected = expect(request).rejects.toMatchObject({ name: "AbortError" });
  controller.abort();
  await rejected;
  expect(fetchSpy).toHaveBeenCalledTimes(1);
});
