import { StrictMode, type ReactNode } from "react";
import { renderHook } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";
import { composerInsertionText, useStablePluginComposerCapability } from "./composer-capability";

function strictWrapper({ children }: { children: ReactNode }) {
  return <StrictMode>{children}</StrictMode>;
}

describe("composerInsertionText", () => {
  it("trims and ignores an empty transcript", () => {
    expect(composerInsertionText("   ", "a")).toBe("");
    expect(composerInsertionText("", "")).toBe("");
  });

  it("adds one space only when the preceding character is not whitespace", () => {
    expect(composerInsertionText("  world  ", "o")).toBe(" world");
    expect(composerInsertionText("world", " ")).toBe("world");
    expect(composerInsertionText("world", "\n")).toBe("world");
    expect(composerInsertionText("world", "")).toBe("world");
  });
});

describe("useStablePluginComposerCapability", () => {
  it("keeps one capability object across re-renders", () => {
    const { result, rerender } = renderHook(
      (submittable: boolean) =>
        useStablePluginComposerCapability({
          insertText: () => true,
          focus: () => true,
          submit: async () => submittable,
        }),
      { initialProps: false },
    );

    const first = result.current;
    rerender(true);
    expect(result.current).toBe(first);
  });

  it("revalidates the native gate at call time rather than at handout time", async () => {
    const { result, rerender } = renderHook(
      (submittable: boolean) =>
        useStablePluginComposerCapability({
          insertText: () => true,
          focus: () => true,
          submit: async () => submittable,
        }),
      { initialProps: false },
    );

    // A plugin that captured the capability while submit was blocked...
    const captured = result.current;
    await expect(captured.submit()).resolves.toEqual({ status: "blocked" });

    // ...still submits once the native form becomes submittable.
    rerender(true);
    await expect(captured.submit()).resolves.toEqual({ status: "submitted" });
  });

  it("survives a StrictMode remount so a long-running plugin action still works", async () => {
    const insertText = vi.fn(() => true);
    const { result } = renderHook(
      () =>
        useStablePluginComposerCapability({
          insertText,
          focus: () => true,
          submit: async () => true,
        }),
      { wrapper: strictWrapper },
    );

    expect(result.current.insertText("hello")).toEqual({ status: "inserted" });
    await expect(result.current.submit()).resolves.toEqual({ status: "submitted" });
    expect(insertText).toHaveBeenCalledWith("hello");
  });

  it("fails closed once the composer unmounts", async () => {
    const insertText = vi.fn(() => true);
    const { result, unmount } = renderHook(() =>
      useStablePluginComposerCapability({
        insertText,
        focus: () => true,
        submit: async () => true,
      }),
    );

    const captured = result.current;
    unmount();

    expect(captured.insertText("hello")).toEqual({ status: "unavailable" });
    expect(captured.focus()).toEqual({ status: "unavailable" });
    await expect(captured.submit()).resolves.toEqual({ status: "unavailable" });
    expect(insertText).not.toHaveBeenCalled();
  });
});
