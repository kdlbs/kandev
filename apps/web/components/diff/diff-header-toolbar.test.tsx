import { describe, it, expect, vi, beforeAll, afterEach } from "vitest";
import { renderHook, render, screen, fireEvent, cleanup } from "@testing-library/react";
import { TooltipProvider } from "@kandev/ui/tooltip";
import type { ReactElement } from "react";

import { useDiffHeaderToolbar } from "./diff-header-toolbar";

const FILE = "src/foo.ts";

afterEach(() => cleanup());

beforeAll(() => {
  // happy-dom's clipboard implementation is partial — stub writeText so the
  // copy-diff handler can be invoked without throwing.
  Object.defineProperty(navigator, "clipboard", {
    value: { writeText: vi.fn().mockResolvedValue(undefined) },
    configurable: true,
  });
});

function renderToolbar(node: ReactElement) {
  return render(<TooltipProvider>{node}</TooltipProvider>);
}

/**
 * Build a synthetic FileDiffMetadata shape. Calling this through the
 * hook callback means a future library change to the metadata shape (rename,
 * removal, or restructure) surfaces here rather than only at the live FileDiff
 * render site.
 */
function headerProp(name: string | undefined) {
  // eslint-disable-next-line @typescript-eslint/no-explicit-any
  return (name === undefined ? {} : { name }) as any;
}

const baseOpts = {
  filePath: FILE,
  diff: `diff --git a/${FILE} b/${FILE}\n--- a/${FILE}\n+++ b/${FILE}\n@@ -1,1 +1,1 @@\n-a\n+b`,
  wordWrap: false,
  onToggleWordWrap: vi.fn(),
  viewMode: "split" as const,
  onToggleViewMode: vi.fn(),
};

describe("useDiffHeaderToolbar", () => {
  it("returns a callback that renders the toolbar buttons", () => {
    const { result } = renderHook(() => useDiffHeaderToolbar(baseOpts));
    // The callback signature is the library's RenderHeaderMetadataProps; we
    // pass a synthetic value so a real change in the library's prop shape
    // would surface here instead of only at the FileDiff render site.
    const node = result.current(headerProp(FILE));
    renderToolbar(node as ReactElement);

    expect(screen.getByLabelText("Copy diff")).toBeTruthy();
    expect(screen.getByLabelText("Toggle word wrap")).toBeTruthy();
    expect(screen.getByLabelText("Switch to unified view")).toBeTruthy();
  });

  it("wires the copy-diff button to navigator.clipboard.writeText", () => {
    const { result } = renderHook(() => useDiffHeaderToolbar(baseOpts));
    const node = result.current(headerProp(FILE));
    renderToolbar(node as ReactElement);

    fireEvent.click(screen.getByLabelText("Copy diff"));
    expect(navigator.clipboard.writeText).toHaveBeenCalledWith(baseOpts.diff);
  });

  it("invokes onToggleWordWrap and onToggleViewMode when their buttons are clicked", () => {
    const onToggleWordWrap = vi.fn();
    const onToggleViewMode = vi.fn();
    const { result } = renderHook(() =>
      useDiffHeaderToolbar({ ...baseOpts, onToggleWordWrap, onToggleViewMode }),
    );
    const node = result.current(headerProp(FILE));
    renderToolbar(node as ReactElement);

    fireEvent.click(screen.getByLabelText("Toggle word wrap"));
    fireEvent.click(screen.getByLabelText("Switch to unified view"));
    expect(onToggleWordWrap).toHaveBeenCalledTimes(1);
    expect(onToggleViewMode).toHaveBeenCalledTimes(1);
  });

  it("prefers the path from the library callback's fileDiff.name over the opts filePath", () => {
    const onOpenFile = vi.fn();
    const { result } = renderHook(() => useDiffHeaderToolbar({ ...baseOpts, onOpenFile }));
    const node = result.current(headerProp("src/renamed.ts"));
    renderToolbar(node as ReactElement);

    fireEvent.click(screen.getByLabelText("Edit"));
    expect(onOpenFile).toHaveBeenCalledWith("src/renamed.ts");
  });

  it("falls back to opts.filePath when the callback prop has no fileDiff.name", () => {
    const onOpenFile = vi.fn();
    const { result } = renderHook(() => useDiffHeaderToolbar({ ...baseOpts, onOpenFile }));
    const node = result.current(headerProp(undefined));
    renderToolbar(node as ReactElement);

    fireEvent.click(screen.getByLabelText("Edit"));
    expect(onOpenFile).toHaveBeenCalledWith(FILE);
  });

  it("shows the markdown preview button only for .md/.mdx files", () => {
    const onPreviewMarkdown = vi.fn();
    const { result } = renderHook(() => useDiffHeaderToolbar({ ...baseOpts, onPreviewMarkdown }));
    const tsNode = result.current(headerProp(FILE));
    const { unmount } = renderToolbar(tsNode as ReactElement);
    expect(screen.queryByLabelText("Preview markdown")).toBeNull();
    unmount();

    const mdNode = result.current(headerProp("README.md"));
    renderToolbar(mdNode as ReactElement);
    expect(screen.getByLabelText("Preview markdown")).toBeTruthy();
  });
});
