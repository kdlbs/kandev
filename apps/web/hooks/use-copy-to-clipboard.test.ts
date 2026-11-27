import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import { renderHook, act, cleanup } from "@testing-library/react";

import { useCopyToClipboard } from "./use-copy-to-clipboard";

const SAMPLE_TEXT = "test clipboard content";
const originalClipboard = Object.getOwnPropertyDescriptor(navigator, "clipboard");
const originalExecCommand = Object.getOwnPropertyDescriptor(document, "execCommand");

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

beforeEach(() => {
  document.body.innerHTML = "";
  execCommandMock = vi.fn().mockReturnValue(false);
  Object.defineProperty(document, "execCommand", { configurable: true, value: execCommandMock });
});

afterEach(() => {
  cleanup();
  vi.useRealTimers();
  vi.restoreAllMocks();
  if (originalClipboard) Object.defineProperty(navigator, "clipboard", originalClipboard);
  else Reflect.deleteProperty(navigator, "clipboard");
  if (originalExecCommand) Object.defineProperty(document, "execCommand", originalExecCommand);
  else Reflect.deleteProperty(document, "execCommand");
});

describe("modern path — navigator.clipboard.writeText available", () => {
  it("calls writeText and sets copied to true", async () => {
    const writeText = vi.fn().mockResolvedValue(undefined);
    Object.defineProperty(navigator, "clipboard", { configurable: true, value: { writeText } });

    const { result } = renderHook(() => useCopyToClipboard());
    await act(async () => {
      await result.current.copy(SAMPLE_TEXT);
    });

    expect(writeText).toHaveBeenCalledWith(SAMPLE_TEXT);
    expect(result.current.copied).toBe(true);
  });

  it("falls back to execCommand when writeText rejects", async () => {
    const writeText = vi.fn().mockRejectedValue(new Error("permission denied"));
    Object.defineProperty(navigator, "clipboard", { configurable: true, value: { writeText } });
    execCommandMock.mockReturnValue(true);

    const { result } = renderHook(() => useCopyToClipboard());
    await act(async () => {
      await result.current.copy(SAMPLE_TEXT);
    });

    expect(writeText).toHaveBeenCalledWith(SAMPLE_TEXT);
    expect(execCommandMock).toHaveBeenCalledWith("copy");
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
    await act(async () => {
      await result.current.copy(SAMPLE_TEXT);
    });

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
    await act(async () => {
      await result.current.copy(SAMPLE_TEXT);
    });

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
    const { container: dlg, trigger } = makeDialogContainer("data-slot", "dialog-content");
    const dialogAppendSpy = vi.spyOn(dlg, "appendChild");
    const bodyAppendSpy = vi.spyOn(document.body, "appendChild");

    const { result } = renderHook(() => useCopyToClipboard());
    await act(async () => {
      await result.current.copy(SAMPLE_TEXT);
    });

    expect(dialogAppendSpy).toHaveBeenCalledWith(expect.any(HTMLTextAreaElement));
    expect(bodyAppendSpy.mock.calls.every(([el]) => !(el instanceof HTMLTextAreaElement))).toBe(
      true,
    );
    expect(result.current.copied).toBe(true);
    expect(document.activeElement).toBe(trigger);
  });

  it("failure: copied stays false and console.error is called when execCommand fails", async () => {
    const errorSpy = vi.spyOn(console, "error").mockImplementation(() => {});

    const { result } = renderHook(() => useCopyToClipboard());
    await act(async () => {
      await result.current.copy(SAMPLE_TEXT);
    });

    expect(result.current.copied).toBe(false);
    expect(errorSpy).toHaveBeenCalledWith("Failed to copy to clipboard");
  });
});

let writeText: ReturnType<typeof vi.fn>;

beforeEach(() => {
  vi.useFakeTimers();
  writeText = vi.fn().mockResolvedValue(undefined);
  Object.defineProperty(navigator, "clipboard", { configurable: true, value: { writeText } });
});

function advance(milliseconds: number) {
  act(() => vi.advanceTimersByTime(milliseconds));
}

function deferredWrite() {
  let resolve!: () => void;
  const promise = new Promise<void>((complete) => {
    resolve = complete;
  });
  return { promise, resolve };
}

describe("useCopyToClipboard: acknowledged success duration", () => {
  // @covers AC-UI-CLIPBOARD-FEEDBACK-001.1, AC-UI-CLIPBOARD-FEEDBACK-001.2
  it.each([undefined, 1000])(
    "renews the full duration after repeated success (%s)",
    async (duration) => {
      const interval = duration ?? 2000;
      const { result } = renderHook(() => useCopyToClipboard(duration));
      await act(async () => {
        await result.current.copy("first exact text");
      });
      expect(result.current.copied).toBe(true);
      advance(interval / 2);
      await act(async () => {
        await result.current.copy("second exact text");
      });
      expect(writeText.mock.calls).toEqual([["first exact text"], ["second exact text"]]);
      advance(interval / 2 - 1);
      expect(result.current.copied).toBe(true);
      advance(1);
      expect(result.current.copied).toBe(true);
      advance(interval / 2 - 1);
      expect(result.current.copied).toBe(true);
      advance(1);
      expect(result.current.copied).toBe(false);
    },
  );

  it("expires an ordinary single success after the default duration", async () => {
    const { result } = renderHook(() => useCopyToClipboard());
    await act(async () => {
      await result.current.copy(SAMPLE_TEXT);
    });
    expect(result.current.copied).toBe(true);
    advance(1999);
    expect(result.current.copied).toBe(true);
    advance(1);
    expect(result.current.copied).toBe(false);
  });

  // @covers AC-UI-CLIPBOARD-FEEDBACK-001.2
  it("clears zero-duration success on the next timer turn", async () => {
    const { result } = renderHook(() => useCopyToClipboard(0));
    await act(async () => {
      await result.current.copy(SAMPLE_TEXT);
    });
    expect(result.current.copied).toBe(true);
    advance(0);
    expect(result.current.copied).toBe(false);
    expect(vi.getTimerCount()).toBe(0);
  });
});

describe("useCopyToClipboard: clipboard completion order", () => {
  // @covers AC-UI-CLIPBOARD-FEEDBACK-001.3
  it("starts feedback at acknowledgement rather than invocation", async () => {
    const pending = deferredWrite();
    writeText.mockReturnValueOnce(pending.promise);
    const { result } = renderHook(() => useCopyToClipboard());
    const copying = result.current.copy("pending exact text");
    advance(3000);
    expect(result.current.copied).toBe(false);
    await act(async () => {
      pending.resolve();
      await copying;
    });
    expect(result.current.copied).toBe(true);
    advance(1999);
    expect(result.current.copied).toBe(true);
    advance(1);
    expect(result.current.copied).toBe(false);
  });

  it("renews when an earlier-started write is acknowledged last", async () => {
    const earlier = deferredWrite();
    writeText.mockReturnValueOnce(earlier.promise);
    const { result } = renderHook(() => useCopyToClipboard());
    const copying = result.current.copy("earlier request");
    await act(async () => {
      await result.current.copy("later request");
    });
    advance(1000);
    await act(async () => {
      earlier.resolve();
      await copying;
    });
    expect(writeText.mock.calls).toEqual([["earlier request"], ["later request"]]);
    advance(1000);
    expect(result.current.copied).toBe(true);
    advance(999);
    expect(result.current.copied).toBe(true);
    advance(1);
    expect(result.current.copied).toBe(false);
  });
});

describe("useCopyToClipboard: failed clipboard feedback", () => {
  // @covers AC-UI-CLIPBOARD-FEEDBACK-001.4
  it("preserves the prior successful deadline when both transports fail", async () => {
    const errorSpy = vi.spyOn(console, "error").mockImplementation(() => {});
    const { result } = renderHook(() => useCopyToClipboard());
    await act(async () => {
      await result.current.copy("successful text");
    });
    advance(1000);
    writeText.mockRejectedValueOnce(new Error("permission denied"));
    await act(async () => {
      await result.current.copy("failed text");
    });
    expect(execCommandMock).toHaveBeenCalledWith("copy");
    expect(errorSpy).toHaveBeenCalledWith("Failed to copy to clipboard");
    expect(result.current.copied).toBe(true);
    advance(999);
    expect(result.current.copied).toBe(true);
    advance(1);
    expect(result.current.copied).toBe(false);
  });
});

describe("useCopyToClipboard: captured feedback duration", () => {
  // @covers AC-UI-CLIPBOARD-FEEDBACK-001.5
  it("changes duration only for a subsequent successful action", async () => {
    const { result, rerender } = renderHook(({ duration }) => useCopyToClipboard(duration), {
      initialProps: { duration: 1000 },
    });
    await act(async () => {
      await result.current.copy("original duration");
    });
    advance(500);
    rerender({ duration: 3000 });
    advance(499);
    expect(result.current.copied).toBe(true);
    advance(1);
    expect(result.current.copied).toBe(false);
    await act(async () => {
      await result.current.copy("new duration");
    });
    advance(2999);
    expect(result.current.copied).toBe(true);
    advance(1);
    expect(result.current.copied).toBe(false);
  });

  it("uses the retained callback duration when its write completes later", async () => {
    const pending = deferredWrite();
    writeText.mockReturnValueOnce(pending.promise);
    const { result, rerender } = renderHook(({ duration }) => useCopyToClipboard(duration), {
      initialProps: { duration: 1000 },
    });
    const retainedCopy = result.current.copy;
    const copying = retainedCopy("retained callback");
    rerender({ duration: 3000 });
    await act(async () => {
      await result.current.copy("current callback");
    });
    advance(500);
    await act(async () => {
      pending.resolve();
      await copying;
    });
    advance(999);
    expect(result.current.copied).toBe(true);
    advance(1);
    expect(result.current.copied).toBe(false);
    advance(1500);
    expect(result.current.copied).toBe(false);
  });
});

describe("useCopyToClipboard: independent feedback and cleanup", () => {
  // @covers AC-UI-CLIPBOARD-FEEDBACK-001.6
  it("keeps separate instances' success deadlines independent", async () => {
    const first = renderHook(() => useCopyToClipboard(1000));
    const second = renderHook(() => useCopyToClipboard());
    await act(async () => {
      await first.result.current.copy("first instance");
    });
    advance(500);
    await act(async () => {
      await second.result.current.copy("second instance");
    });
    advance(500);
    expect(first.result.current.copied).toBe(false);
    expect(second.result.current.copied).toBe(true);
    advance(1499);
    expect(second.result.current.copied).toBe(true);
    advance(1);
    expect(second.result.current.copied).toBe(false);
  });

  it.each([false, true])("releases scheduled work on unmount (renewed: %s)", async (renewed) => {
    const first = renderHook(() => useCopyToClipboard());
    const second = renderHook(() => useCopyToClipboard(4000));
    await act(async () => {
      await first.result.current.copy("first instance");
      await second.result.current.copy("surviving instance");
    });
    advance(500);
    if (renewed) {
      await act(async () => {
        await first.result.current.copy("replacement success");
      });
    }
    first.unmount();
    expect(vi.getTimerCount()).toBe(1);
    advance(3499);
    expect(second.result.current.copied).toBe(true);
    advance(1);
    expect(second.result.current.copied).toBe(false);
    expect(vi.getTimerCount()).toBe(0);
  });
});
