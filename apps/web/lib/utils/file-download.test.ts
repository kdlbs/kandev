import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { triggerFileDownload } from "./file-download";

const createObjectURLMock = vi.fn((_blob: Blob): string => "blob:mock");
const revokeObjectURLMock = vi.fn((_url: string): void => undefined);
const originalCreateObjectURL = URL.createObjectURL;
const originalRevokeObjectURL = URL.revokeObjectURL;

beforeEach(() => {
  URL.createObjectURL = createObjectURLMock as unknown as typeof URL.createObjectURL;
  URL.revokeObjectURL = revokeObjectURLMock as unknown as typeof URL.revokeObjectURL;
  createObjectURLMock.mockClear();
  revokeObjectURLMock.mockClear();
});

afterEach(() => {
  URL.createObjectURL = originalCreateObjectURL;
  URL.revokeObjectURL = originalRevokeObjectURL;
  vi.restoreAllMocks();
});

function spyAnchorClick(): { click: ReturnType<typeof vi.fn>; download: () => string } {
  const click = vi.fn();
  const originalCreate = document.createElement.bind(document);
  let capturedAnchor: HTMLAnchorElement | null = null;
  vi.spyOn(document, "createElement").mockImplementation(
    (tagName: string, options?: ElementCreationOptions) => {
      const el = originalCreate(tagName as keyof HTMLElementTagNameMap, options);
      if (tagName === "a") {
        const anchor = el as HTMLAnchorElement;
        anchor.click = click;
        capturedAnchor = anchor;
      }
      return el;
    },
  );
  return { click, download: () => capturedAnchor?.download ?? "" };
}

describe("triggerFileDownload", () => {
  it("creates a text blob and clicks a link with the file name", () => {
    const { click } = spyAnchorClick();

    triggerFileDownload({ fileName: "hello.txt", content: "hi", isBinary: false });

    expect(createObjectURLMock).toHaveBeenCalledTimes(1);
    const blob = createObjectURLMock.mock.calls[0]?.[0] as unknown as Blob;
    expect(blob).toBeInstanceOf(Blob);
    expect(blob.type).toContain("text/plain");
    expect(click).toHaveBeenCalledTimes(1);
    expect(revokeObjectURLMock).toHaveBeenCalledWith("blob:mock");
  });

  it("decodes base64 content when isBinary is true", () => {
    spyAnchorClick();

    triggerFileDownload({ fileName: "hello.bin", content: btoa("hi"), isBinary: true });

    const blob = createObjectURLMock.mock.calls[0]?.[0] as unknown as Blob;
    expect(blob).toBeInstanceOf(Blob);
    expect(blob.type).toBe("application/octet-stream");
    // "hi" -> 2 bytes
    expect(blob.size).toBe(2);
  });

  it("sets the download attribute to just the file name (basename)", () => {
    const { download } = spyAnchorClick();

    triggerFileDownload({ fileName: "src/lib/notes.md", content: "hi", isBinary: false });

    expect(download()).toBe("notes.md");
  });
});
