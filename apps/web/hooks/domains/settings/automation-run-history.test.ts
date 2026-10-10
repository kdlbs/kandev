import { describe, expect, it, vi } from "vitest";
import { collectCursorPages } from "./automation-run-history";

describe("collectCursorPages", () => {
  it("stops when a cursor repeats", async () => {
    const loadPage = vi
      .fn()
      .mockResolvedValueOnce({ items: [1], next_cursor: "a" })
      .mockResolvedValueOnce({ items: [2], next_cursor: "a" });

    await expect(collectCursorPages(loadPage)).resolves.toEqual([1, 2]);
    expect(loadPage).toHaveBeenCalledTimes(2);
  });

  it("caps the number of pages even when the server keeps returning cursors", async () => {
    const loadPage = vi.fn(async (cursor?: string) => ({
      items: [cursor ?? "first"],
      next_cursor: String(loadPage.mock.calls.length),
    }));

    await expect(collectCursorPages(loadPage, 3)).resolves.toEqual(["first", "1", "2"]);
    expect(loadPage).toHaveBeenCalledTimes(3);
  });
});
