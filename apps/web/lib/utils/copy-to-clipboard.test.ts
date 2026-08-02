import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { copyToClipboard } from "./copy-to-clipboard";

const SAMPLE_TEXT = "test clipboard content";

describe("copyToClipboard", () => {
  beforeEach(() => {
    document.body.innerHTML = "";
    Object.defineProperty(document, "execCommand", {
      configurable: true,
      value: vi.fn().mockReturnValue(false),
    });
    Object.defineProperty(navigator, "clipboard", {
      configurable: true,
      value: undefined,
    });
  });

  afterEach(() => {
    vi.restoreAllMocks();
  });

  it("prefers the modern Clipboard API", async () => {
    const writeText = vi.fn().mockResolvedValue(undefined);
    Object.defineProperty(navigator, "clipboard", {
      configurable: true,
      value: { writeText },
    });
    const execCommand = vi.mocked(document.execCommand);

    await expect(copyToClipboard(SAMPLE_TEXT)).resolves.toBe(true);

    expect(writeText).toHaveBeenCalledWith(SAMPLE_TEXT);
    expect(execCommand).not.toHaveBeenCalled();
  });

  it("uses the DOM fallback when the Clipboard API is unavailable", async () => {
    const execCommand = vi.mocked(document.execCommand).mockReturnValue(true);

    await expect(copyToClipboard(SAMPLE_TEXT)).resolves.toBe(true);

    expect(execCommand).toHaveBeenCalledWith("copy");
    expect(document.querySelector("textarea")).toBeNull();
  });

  it("uses the DOM fallback when the Clipboard API rejects and restores dialog focus", async () => {
    const writeText = vi.fn().mockRejectedValue(new Error("permission denied"));
    Object.defineProperty(navigator, "clipboard", {
      configurable: true,
      value: { writeText },
    });
    const execCommand = vi.mocked(document.execCommand).mockReturnValue(true);
    const dialog = document.createElement("div");
    dialog.setAttribute("role", "dialog");
    const trigger = document.createElement("button");
    dialog.appendChild(trigger);
    document.body.appendChild(dialog);
    trigger.focus();

    await expect(copyToClipboard(SAMPLE_TEXT)).resolves.toBe(true);

    expect(writeText).toHaveBeenCalledWith(SAMPLE_TEXT);
    expect(execCommand).toHaveBeenCalledWith("copy");
    expect(dialog.querySelector("textarea")).toBeNull();
    expect(document.activeElement).toBe(trigger);
  });

  it("returns false when both copy mechanisms fail", async () => {
    const execCommand = vi.mocked(document.execCommand).mockReturnValue(false);

    await expect(copyToClipboard(SAMPLE_TEXT)).resolves.toBe(false);

    expect(execCommand).toHaveBeenCalledWith("copy");
  });
});
