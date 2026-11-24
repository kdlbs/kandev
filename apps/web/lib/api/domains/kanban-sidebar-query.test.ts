import { afterEach, expect, it, vi } from "vitest";
import { querySidebarTasks } from "./kanban-api";
import type { SidebarTaskQuery } from "@/lib/types/http";

const query: SidebarTaskQuery = {
  group: "plugin:acme:labels",
  disclosure_view_id: "saved",
  collapsed_group_keys: [],
  collapsed_task_ids: [],
  filters: [],
  sort: { key: "title", direction: "asc" },
  page: 1,
  page_size: 100,
  locale: "en",
};
afterEach(() => vi.unstubAllGlobals());

it.each([1, 1001])(
  "keeps bounded client disclosure independent of concurrent saved settings: %i",
  async (count) => {
    const keys = Array.from(
      { length: count },
      (_, i) => `plugin-group:${i.toString(16).padStart(64, "0")}`,
    );
    const fetch = vi.fn(async (_input: unknown, options?: RequestInit) => {
      const body = JSON.parse(String(options?.body));
      expect(body.collapsed_group_keys).toEqual(keys);
      expect(body.disclosure_view_id).toBeUndefined();
      return new Response(JSON.stringify({ entries: [] }));
    });
    vi.stubGlobal("fetch", fetch);
    await querySidebarTasks("ws", { ...query, collapsed_group_keys: keys });
    expect(fetch).toHaveBeenCalledTimes(1);
  },
);

it("recovers an oversized saved disclosure through the bounded public request", async () => {
  const keys = Array.from(
    { length: 6000 },
    (_, i) => `plugin-group:${i.toString(16).padStart(64, "0")}`,
  );
  vi.stubGlobal(
    "fetch",
    vi.fn(async (_input: unknown, options?: RequestInit) => {
      const raw = String(options?.body);
      if (new TextEncoder().encode(raw).byteLength > 256 * 1024)
        return new Response("too large", { status: 413 });
      const body = JSON.parse(raw);
      expect(body.disclosure_view_id).toBe("saved");
      expect(body.collapsed_group_keys).toEqual([]);
      return new Response(JSON.stringify({ query_key: "authorized", entries: [] }));
    }),
  );
  await expect(
    querySidebarTasks("ws", { ...query, collapsed_group_keys: keys }),
  ).resolves.toMatchObject({ query_key: "authorized" });
});
