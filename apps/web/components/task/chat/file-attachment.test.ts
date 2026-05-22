import { afterEach, describe, expect, it, vi } from "vitest";
import { processFile } from "./file-attachment";

const UUID_V4_REGEX = /^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$/;

// Minimal FileReader stub — jsdom's version is fine, but we stub for deterministic output.
class FakeFileReader {
  public result: string | null = null;
  public onload: ((event: { target: { result: string } }) => void) | null = null;
  public onerror: (() => void) | null = null;
  readAsDataURL(file: File) {
    // Small synthetic data URL — content doesn't matter for the id assertion.
    const payload = "AAAA"; // 3 bytes base64
    this.result = `data:${file.type || "application/octet-stream"};base64,${payload}`;
    queueMicrotask(() => this.onload?.({ target: { result: this.result as string } }));
  }
}

// Image stub — fires onload synchronously so the image branch resolves.
class FakeImage {
  public onload: (() => void) | null = null;
  public onerror: (() => void) | null = null;
  set src(_: string) {
    queueMicrotask(() => this.onload?.());
  }
}

describe("processFile in insecure context (HTTP, no crypto.randomUUID)", () => {
  afterEach(() => {
    vi.unstubAllGlobals();
  });

  it("assigns a fallback UUID to non-image attachments when crypto.randomUUID is unavailable", async () => {
    vi.stubGlobal("crypto", {}); // simulate non-secure context
    vi.stubGlobal("FileReader", FakeFileReader);

    const file = new File(["hello"], "notes.txt", { type: "text/plain" });
    Object.defineProperty(file, "size", { value: 5 });

    const attachment = await processFile(file);
    expect(attachment).not.toBeNull();
    expect(attachment!.id).toMatch(UUID_V4_REGEX);
    expect(attachment!.isImage).toBe(false);
    expect(attachment!.fileName).toBe("notes.txt");
  });

  it("assigns a fallback UUID to image attachments when crypto.randomUUID is unavailable", async () => {
    vi.stubGlobal("crypto", {});
    vi.stubGlobal("FileReader", FakeFileReader);
    vi.stubGlobal("Image", FakeImage);

    const file = new File(["img"], "shot.png", { type: "image/png" });
    Object.defineProperty(file, "size", { value: 3 });

    const attachment = await processFile(file);
    expect(attachment).not.toBeNull();
    expect(attachment!.id).toMatch(UUID_V4_REGEX);
    expect(attachment!.isImage).toBe(true);
    expect(attachment!.preview).toMatch(/^data:image\/png;base64,/);
  });
});
