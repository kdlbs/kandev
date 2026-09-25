import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { triggerBlobDownload, triggerFileDownload } from "./file-download";
import {
  completeNativeBlobDownload,
  MAX_NATIVE_BLOB_DOWNLOAD_AGE_MS,
} from "./native-blob-download";

const createObjectURLMock = vi.fn((_blob: Blob): string => "blob:mock");
const revokeObjectURLMock = vi.fn((_url: string): void => undefined);
const SAMPLE_BLOB_CONTENT = "bytes";
const SAMPLE_DOWNLOAD_NAME = "report.zip";
const originalCreateObjectURL = URL.createObjectURL;
const originalRevokeObjectURL = URL.revokeObjectURL;
const originalTauriInternals = Object.getOwnPropertyDescriptor(window, "__TAURI_INTERNALS__");

beforeEach(() => {
  vi.useFakeTimers();
  URL.createObjectURL = createObjectURLMock as unknown as typeof URL.createObjectURL;
  URL.revokeObjectURL = revokeObjectURLMock as unknown as typeof URL.revokeObjectURL;
  createObjectURLMock.mockClear();
  revokeObjectURLMock.mockClear();
});

afterEach(() => {
  vi.runOnlyPendingTimers();
  vi.useRealTimers();
  URL.createObjectURL = originalCreateObjectURL;
  URL.revokeObjectURL = originalRevokeObjectURL;
  if (originalTauriInternals) {
    Object.defineProperty(window, "__TAURI_INTERNALS__", originalTauriInternals);
  } else {
    Reflect.deleteProperty(window, "__TAURI_INTERNALS__");
  }
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
    expect(revokeObjectURLMock).not.toHaveBeenCalled();
    vi.runOnlyPendingTimers();
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

describe("triggerBlobDownload", () => {
  it("downloads an already-built blob under the exact given file name", () => {
    const { click, download } = spyAnchorClick();
    const blob = new Blob(["zip-bytes"], { type: "application/zip" });

    triggerBlobDownload(blob, "kandev-automations.zip");

    expect(createObjectURLMock).toHaveBeenCalledWith(blob);
    expect(click).toHaveBeenCalledTimes(1);
    expect(download()).toBe("kandev-automations.zip");
    expect(revokeObjectURLMock).not.toHaveBeenCalled();
    vi.runOnlyPendingTimers();
    expect(revokeObjectURLMock).toHaveBeenCalledWith("blob:mock");
  });

  it("keeps native Blob URLs alive through delayed initiation and until a terminal event", () => {
    Object.defineProperty(window, "__TAURI_INTERNALS__", {
      configurable: true,
      value: { invoke: vi.fn(), transformCallback: vi.fn() },
    });
    const { click } = spyAnchorClick();

    triggerBlobDownload(new Blob([SAMPLE_BLOB_CONTENT]), SAMPLE_DOWNLOAD_NAME);
    vi.advanceTimersByTime(120_000);
    expect(click).toHaveBeenCalledOnce();
    expect(revokeObjectURLMock).not.toHaveBeenCalled();

    completeNativeBlobDownload({
      status: "started",
      url: "blob:mock",
      fileName: SAMPLE_DOWNLOAD_NAME,
    });
    vi.advanceTimersByTime(60_000);
    expect(revokeObjectURLMock).not.toHaveBeenCalled();

    completeNativeBlobDownload({
      status: "saved",
      url: "blob:mock",
      fileName: SAMPLE_DOWNLOAD_NAME,
    });
    expect(revokeObjectURLMock).toHaveBeenCalledOnce();
    expect(revokeObjectURLMock).toHaveBeenCalledWith("blob:mock");
  });

  it("bounds cleanup if native completion feedback never arrives", () => {
    Object.defineProperty(window, "__TAURI_INTERNALS__", {
      configurable: true,
      value: { invoke: vi.fn(), transformCallback: vi.fn() },
    });
    spyAnchorClick();

    triggerBlobDownload(new Blob([SAMPLE_BLOB_CONTENT]), SAMPLE_DOWNLOAD_NAME);
    vi.advanceTimersByTime(MAX_NATIVE_BLOB_DOWNLOAD_AGE_MS);

    expect(revokeObjectURLMock).toHaveBeenCalledWith("blob:mock");
  });
});
