import { afterEach, describe, expect, it, vi } from "vitest";
import { readClipboardAttachments } from "./clipboard-attachments";

afterEach(() => {
  vi.restoreAllMocks();
});

function clipboardData(html: string, plainText = ""): DataTransfer {
  return {
    files: [],
    items: [],
    getData: (type: string) => (type === "text/html" ? html : plainText),
  } as unknown as DataTransfer;
}

describe("readClipboardAttachments", () => {
  it("detects image-only HTML without parsing clipboard markup into a DOM", () => {
    const parseFromString = vi.spyOn(DOMParser.prototype, "parseFromString");

    expect(
      readClipboardAttachments(clipboardData('<img src="https://example.test/image.png">')),
    ).toEqual({
      files: [],
      issue: "unreadable-image",
    });
    expect(parseFromString).not.toHaveBeenCalled();
  });

  it("leaves image HTML with visible clipboard text to the editor", () => {
    expect(
      readClipboardAttachments(
        clipboardData(
          '<p>Visible caption <img src="https://example.test/image.png"></p>',
          "Visible caption",
        ),
      ),
    ).toEqual({ files: [] });
  });

  it.each(["<IMG>", "<img/>", "<img\nsrc='https://example.test/image.png'>"])(
    "recognizes an image tag in %s",
    (html) => {
      expect(readClipboardAttachments(clipboardData(html))).toEqual({
        files: [],
        issue: "unreadable-image",
      });
    },
  );

  it.each(["<image>", "<imgfoo>"])("does not mistake %s for an image tag", (html) => {
    expect(readClipboardAttachments(clipboardData(html))).toEqual({ files: [] });
  });
});
