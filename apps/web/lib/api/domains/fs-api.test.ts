import { afterEach, describe, expect, it, vi } from "vitest";
import { createDirectory, listDirectory } from "./fs-api";

const originalFetch = global.fetch;

afterEach(() => {
  global.fetch = originalFetch;
});

describe("listDirectory", () => {
  // @covers AC-WORKSPACES-HIDDEN-FOLDERS-001.2
  it("omits the reveal input by default so the request stays unchanged", async () => {
    const fetchSpy = vi.fn(listingResponse([]));
    global.fetch = fetchSpy as typeof global.fetch;

    await listDirectory("/home/user");

    expect(requestUrl(fetchSpy)).toBe("/api/v1/fs/list-dir?path=%2Fhome%2Fuser");
  });

  // @covers AC-WORKSPACES-HIDDEN-FOLDERS-001.1
  it("asks the backend to reveal hidden directories only when requested", async () => {
    const fetchSpy = vi.fn(listingResponse([]));
    global.fetch = fetchSpy as typeof global.fetch;

    await listDirectory("/home/user", { includeHidden: true });

    expect(requestUrl(fetchSpy)).toBe(
      "/api/v1/fs/list-dir?path=%2Fhome%2Fuser&include_hidden=true",
    );
  });

  it("keeps the empty path request addressable when the reveal is requested", async () => {
    const fetchSpy = vi.fn(listingResponse([]));
    global.fetch = fetchSpy as typeof global.fetch;

    await listDirectory("", { includeHidden: true });

    // The backend defaults an empty path to $HOME, so the request must stay
    // addressable rather than degrade into a bare query string.
    expect(requestUrl(fetchSpy)).toBe("/api/v1/fs/list-dir?include_hidden=true");
  });
});

function listingResponse(entries: Array<{ name: string; path: string }>) {
  return async () =>
    new Response(
      JSON.stringify({ path: "/home/user", parent: "/home", entries, choosable: true }),
      { status: 200, headers: { "Content-Type": "application/json" } },
    );
}

function requestUrl(fetchSpy: ReturnType<typeof vi.fn>): string {
  const [url] = fetchSpy.mock.calls[0] as [string];
  return url.replace("http://localhost:3000", "");
}

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
