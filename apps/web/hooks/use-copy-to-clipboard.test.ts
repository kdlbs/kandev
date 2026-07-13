import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import { renderHook, act } from "@testing-library/react";

import { useCopyToClipboard } from "./use-copy-to-clipboard";

const SAMPLE_TEXT = "test clipboard content";

let execCommandMock: ReturnType<typeof vi.fn>;

function makeDialogContainer(attr: string, value: string) {
  const container = document.createElement("div");
  container.setAttribute(attr, value);
  document.body.appendChild(container);
  const trigger = document.createElement("button");
  container.appendChild(trigger);
  trigger.focus();
  return { container, trigger };
}

describe("useCopyToClipboard", () => {
  beforeEach(() => {
    document.body.innerHTML = "";
    execCommandMock = vi.fn().mockReturnValue(false);
    Object.defineProperty(document, "execCommand", { configurable: true, value: execCommandMock });
  });

  afterEach(() => {
    vi.restoreAllMocks();
  });

  describe("modern path — navigator.clipboard.writeText available", () => {
    it("calls writeText and sets copied to true", async () => {
      const writeText = vi.fn().mockResolvedValue(undefined);
      Object.defineProperty(navigator, "clipboard", { configurable: true, value: { writeText } });

      const { result } = renderHook(() => useCopyToClipboard());
      await act(async () => { await result.current.copy(SAMPLE_TEXT); });

      expect(writeText).toHaveBeenCalledWith(SAMPLE_TEXT);
      expect(result.current.copied).toBe(true);
    });
  });

  describe("fallback path — navigator.clipboard unavailable", () => {
    beforeEach(() => {
      Object.defineProperty(navigator, "clipboard", { configurable: true, value: undefined });
    });

    it("no modal: appends temp textarea to document.body", async () => {
      execCommandMock.mockReturnValue(true);
      const appendSpy = vi.spyOn(document.body, "appendChild");

      const { result } = renderHook(() => useCopyToClipboard());
      await act(async () => { await result.current.copy(SAMPLE_TEXT); });

      expect(appendSpy.mock.calls.some(([el]) => el instanceof HTMLTextAreaElement)).toBe(true);
      expect(result.current.copied).toBe(true);
      expect(document.body.querySelector("textarea")).toBeNull();
    });

    it("in modal (role=dialog): appends temp textarea inside dialog container, not body", async () => {
      execCommandMock.mockReturnValue(true);
      const { container: dialog, trigger } = makeDialogContainer("role", "dialog");
      const dialogAppendSpy = vi.spyOn(dialog, "appendChild");
      const bodyAppendSpy = vi.spyOn(document.body, "appendChild");

      const { result } = renderHook(() => useCopyToClipboard());
      await act(async () => { await result.current.copy(SAMPLE_TEXT); });

      expect(dialogAppendSpy).toHaveBeenCalledWith(expect.any(HTMLTextAreaElement));
      expect(bodyAppendSpy.mock.calls.every(([el]) => !(el instanceof HTMLTextAreaElement))).toBe(
        true,
      );
      expect(result.current.copied).toBe(true);
      expect(dialog.querySelector("textarea")).toBeNull();
      expect(document.activeElement).toBe(trigger);
    });

    it("in modal (data-slot=dialog-content): appends temp textarea inside dialog-content", async () => {
      execCommandMock.mockReturnValue(true);
      const { container: dlg } = makeDialogContainer("data-slot", "dialog-content");
      const dialogAppendSpy = vi.spyOn(dlg, "appendChild");
      const bodyAppendSpy = vi.spyOn(document.body, "appendChild");

      const { result } = renderHook(() => useCopyToClipboard());
      await act(async () => { await result.current.copy(SAMPLE_TEXT); });

      expect(dialogAppendSpy).toHaveBeenCalledWith(expect.any(HTMLTextAreaElement));
      expect(bodyAppendSpy.mock.calls.every(([el]) => !(el instanceof HTMLTextAreaElement))).toBe(
        true,
      );
      expect(result.current.copied).toBe(true);
    });

    it("failure: copied stays false and console.error is called when execCommand fails", async () => {
      const errorSpy = vi.spyOn(console, "error").mockImplementation(() => {});

      const { result } = renderHook(() => useCopyToClipboard());
      await act(async () => { await result.current.copy(SAMPLE_TEXT); });

      expect(result.current.copied).toBe(false);
      expect(errorSpy).toHaveBeenCalledWith("Failed to copy to clipboard");
    });
  });
});

