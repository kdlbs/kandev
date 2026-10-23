import { act, renderHook } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";
import type { MarkdownFileMode } from "./markdown-file-mode";
import { useMarkdownSourcePosition } from "./use-markdown-source-position";

const SOURCE_POSITION = { selection: { anchor: 12, active: 4 }, nearestLine: 2 };
const initialProps = {
  identity: "session/repo/file",
  draftIdentity: Symbol("draft"),
  content: "alpha\r\nbravo\r\ncharlie",
  mode: "source" as MarkdownFileMode,
};
function port() {
  return { capture: vi.fn(() => SOURCE_POSITION), restore: vi.fn() };
}

describe("Markdown mode position coordinator (AC-UI-MARKDOWN-FILE-EDITING-002.4)", () => {
  it("captures before the mode changes and restores only once the destination registers ready", () => {
    const source = port();
    const destination = port();
    const onModeChange = vi.fn(() => expect(source.capture).toHaveBeenCalledOnce());
    const { result, rerender } = renderHook(
      (props) => useMarkdownSourcePosition({ ...props, onModeChange }),
      { initialProps },
    );
    act(() => result.current.sourcePositionHandles.source(source));
    act(() => result.current.handleModeChange("preview"));
    expect(destination.restore).not.toHaveBeenCalled();
    rerender({ ...initialProps, mode: "preview" });
    act(() => result.current.sourcePositionHandles.preview(destination));
    expect(destination.restore).toHaveBeenCalledExactlyOnceWith(SOURCE_POSITION);
    act(() => result.current.sourcePositionHandles.preview(destination));
    rerender({ ...initialProps, mode: "preview", content: initialProps.content + "!" });
    expect(destination.restore).toHaveBeenCalledOnce();
  });

  it.each([
    { identity: "another-session/repo/file" },
    { identity: "session/another-repo/file" },
    { identity: "session/repo/another-file" },
    { draftIdentity: Symbol("reopened draft") },
    { content: "accepted replacement" },
  ])("rejects a pending restoration after ownership or source replacement: %o", (change) => {
    const source = port();
    const destination = port();
    const { result, rerender } = renderHook(
      (props) => useMarkdownSourcePosition({ ...props, onModeChange: vi.fn() }),
      { initialProps },
    );
    act(() => result.current.sourcePositionHandles.source(source));
    act(() => result.current.handleModeChange("edit"));
    const staleRegister = result.current.sourcePositionHandles.edit;
    rerender({ ...initialProps, ...change, mode: "edit" });
    act(() => staleRegister(destination));
    act(() => result.current.sourcePositionHandles.edit(destination));
    expect(destination.restore).not.toHaveBeenCalled();
  });

  it("does not replay a superseded transition or a registration after unmount", () => {
    const source = port();
    const edit = port();
    const preview = port();
    const { result, rerender, unmount } = renderHook(
      (props) => useMarkdownSourcePosition({ ...props, onModeChange: vi.fn() }),
      { initialProps },
    );
    act(() => result.current.sourcePositionHandles.source(source));
    act(() => result.current.handleModeChange("edit"));
    rerender({ ...initialProps, mode: "edit" });
    act(() => result.current.handleModeChange("preview"));
    rerender({ ...initialProps, mode: "preview" });
    act(() => result.current.sourcePositionHandles.edit(edit));
    expect(edit.restore).not.toHaveBeenCalled();
    const staleRegister = result.current.sourcePositionHandles.preview;
    unmount();
    staleRegister(preview);
    expect(preview.restore).not.toHaveBeenCalled();
  });

  it("clamps the captured source range after an accepted replacement", () => {
    const source = port();
    const destination = port();
    const { result, rerender } = renderHook(
      (props) => useMarkdownSourcePosition({ ...props, onModeChange: vi.fn() }),
      { initialProps: { ...initialProps, content: "x" } },
    );
    act(() => result.current.sourcePositionHandles.source(source));
    act(() => result.current.handleModeChange("edit"));
    rerender({ ...initialProps, content: "x", mode: "edit" });
    act(() => result.current.sourcePositionHandles.edit(destination));
    expect(destination.restore).toHaveBeenCalledExactlyOnceWith({
      selection: { anchor: 1, active: 1 },
      nearestLine: 1,
    });
  });
});
