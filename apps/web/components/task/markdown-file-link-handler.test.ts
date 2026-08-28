import { act, renderHook } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";

const routerPush = vi.hoisted(() => vi.fn());
const openExternalLink = vi.hoisted(() => vi.fn());

vi.mock("@/lib/routing/client-router", () => ({
  useRouter: () => ({ push: routerPush }),
}));

vi.mock("@/lib/desktop/external-links", () => ({
  openExternalLink,
}));

import { useMarkdownFileLinkHandler } from "./markdown-file-link-handler";

describe("useMarkdownFileLinkHandler", () => {
  beforeEach(() => {
    routerPush.mockReset();
    openExternalLink.mockReset();
  });

  it("resolves a relative repository link from the current Markdown file", () => {
    const onOpenFile = vi.fn();
    const { result } = renderHook(() =>
      useMarkdownFileLinkHandler({
        path: "docs/readme.md",
        worktreePath: "/tmp/repository",
        onOpenFile,
      }),
    );

    act(() => result.current("./guide.md"));

    expect(onOpenFile).toHaveBeenCalledWith("docs/guide.md");
    expect(routerPush).not.toHaveBeenCalled();
  });

  it("routes internal task links through the Kandev client router", () => {
    const { result } = renderHook(() =>
      useMarkdownFileLinkHandler({
        path: "README.md",
        onOpenFile: vi.fn(),
      }),
    );

    act(() => result.current("/t/task-42?layout=advanced"));

    expect(routerPush).toHaveBeenCalledWith("/t/task-42?layout=advanced");
  });

  it("uses the audited external-link opener for external URLs", () => {
    openExternalLink.mockResolvedValue("browser");
    const { result } = renderHook(() =>
      useMarkdownFileLinkHandler({
        path: "README.md",
        onOpenFile: vi.fn(),
      }),
    );

    act(() => result.current("https://example.com/docs"));

    expect(openExternalLink).toHaveBeenCalledWith("https://example.com/docs");
    expect(routerPush).not.toHaveBeenCalled();
  });
});
