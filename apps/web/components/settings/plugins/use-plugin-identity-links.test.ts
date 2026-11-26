import { act, renderHook, waitFor } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import type { PluginIdentityLink } from "@/lib/types/plugins";

const listPluginIdentityLinks =
  vi.fn<(id: string, scope: "own" | "all") => Promise<PluginIdentityLink[]>>();
const createPluginIdentityLinkCode = vi.fn();
const revokePluginIdentityLink = vi.fn<(id: string, linkId: string) => Promise<void>>();

vi.mock("@/lib/api/domains/plugins-api", () => ({
  listPluginIdentityLinks: (id: string, scope: "own" | "all") => listPluginIdentityLinks(id, scope),
  createPluginIdentityLinkCode: (id: string) => createPluginIdentityLinkCode(id),
  revokePluginIdentityLink: (id: string, linkId: string) => revokePluginIdentityLink(id, linkId),
}));

import { usePluginIdentityLinks } from "./use-plugin-identity-links";

function link(id: string): PluginIdentityLink {
  return {
    id,
    provider: "discord",
    subject: `subject-${id}`,
    display_name: id,
    user_id: "user-1",
    user_display_name: "User",
    created_at: "2026-11-22T10:00:00Z",
  };
}

describe("usePluginIdentityLinks", () => {
  beforeEach(() => {
    listPluginIdentityLinks.mockReset();
    createPluginIdentityLinkCode.mockReset();
    revokePluginIdentityLink.mockReset();
  });

  it("lists every link for an admin and only the caller's links otherwise", async () => {
    listPluginIdentityLinks.mockResolvedValue([link("a")]);
    renderHook(() => usePluginIdentityLinks("chat", true));
    await waitFor(() => expect(listPluginIdentityLinks).toHaveBeenCalledWith("chat", "all"));
    renderHook(() => usePluginIdentityLinks("chat", false));
    await waitFor(() => expect(listPluginIdentityLinks).toHaveBeenCalledWith("chat", "own"));
  });

  it("refreshes the list when the window regains focus after a link was made elsewhere", async () => {
    // Read the current server state on every call: mount effects may run more
    // than once, so counting calls would be brittle.
    let serverLinks: PluginIdentityLink[] = [];
    listPluginIdentityLinks.mockImplementation(async () => serverLinks);
    const { result } = renderHook(() => usePluginIdentityLinks("chat", false));
    await waitFor(() => expect(result.current.loading).toBe(false));
    expect(result.current.links).toEqual([]);

    serverLinks = [link("a")];
    await act(async () => {
      window.dispatchEvent(new Event("focus"));
    });
    await waitFor(() => expect(result.current.links.map((l) => l.id)).toEqual(["a"]));
    expect(result.current.loading).toBe(false);
  });

  it("keeps a link listed when revoking it fails", async () => {
    listPluginIdentityLinks.mockResolvedValue([link("a"), link("b")]);
    revokePluginIdentityLink.mockRejectedValueOnce(new Error("offline"));
    revokePluginIdentityLink.mockResolvedValueOnce(undefined);
    const { result } = renderHook(() => usePluginIdentityLinks("chat", false));
    await waitFor(() => expect(result.current.links).toHaveLength(2));

    await act(() => result.current.revoke("a"));
    expect(result.current.links.map((l) => l.id)).toEqual(["a", "b"]);
    expect(result.current.errorKey).toBe("plugins:identityLinkRevokeFailed");

    await act(() => result.current.revoke("a"));
    expect(result.current.links.map((l) => l.id)).toEqual(["b"]);
    expect(result.current.errorKey).toBeNull();
  });
});
