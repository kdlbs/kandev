import { afterEach, describe, expect, it, vi } from "vitest";
import { createDirectory } from "./fs-api";

const originalFetch = global.fetch;

afterEach(() => {
  global.fetch = originalFetch;
});

describe("createDirectory", () => {
  it("posts the folder name and parent path", async () => {
    const fetchSpy = vi.fn(
      async () =>
        new Response(
          JSON.stringify({
            path: "/work/projects",
            parent: "/work",
            entries: [],
            choosable: true,
          }),
          { status: 201, headers: { "Content-Type": "application/json" } },
        ),
    );
    global.fetch = fetchSpy as typeof global.fetch;

    const listing = await createDirectory("/work", "projects");

    expect(fetchSpy).toHaveBeenCalledWith(
      "http://localhost:3000/api/v1/fs/create-dir",
      expect.objectContaining({
        method: "POST",
        body: JSON.stringify({ parent_path: "/work", name: "projects" }),
      }),
    );
    expect(listing).toMatchObject({ path: "/work/projects", choosable: true });
  });
});
