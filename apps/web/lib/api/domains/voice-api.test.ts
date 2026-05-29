import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { ApiError } from "../client";
import { transcribeAudio } from "./voice-api";

const originalFetch = global.fetch;

describe("transcribeAudio", () => {
  afterEach(() => {
    global.fetch = originalFetch;
  });

  beforeEach(() => {
    vi.restoreAllMocks();
  });

  it("posts multipart/form-data with the audio under the 'audio' field", async () => {
    let capturedRequest: { method?: string; bodyText: string } = { bodyText: "" };
    global.fetch = vi.fn(async (_url: RequestInfo | URL, init?: RequestInit) => {
      capturedRequest = {
        method: init?.method,
        bodyText: init?.body instanceof FormData ? "<formdata>" : String(init?.body),
      };
      return new Response(JSON.stringify({ text: "hi" }), {
        status: 200,
        headers: { "Content-Type": "application/json" },
      });
    }) as unknown as typeof fetch;

    const blob = new Blob([new Uint8Array([1, 2, 3])], { type: "audio/webm" });
    const result = await transcribeAudio(blob, "clip.webm", {
      baseUrl: "http://example.test",
    });

    expect(result.text).toBe("hi");
    expect(capturedRequest.method).toBe("POST");
    expect(capturedRequest.bodyText).toBe("<formdata>");
  });

  it("throws ApiError(503) when the server reports not-configured", async () => {
    global.fetch = vi.fn(
      async () =>
        new Response(JSON.stringify({ error: "voice transcription is not configured" }), {
          status: 503,
        }),
    ) as unknown as typeof fetch;

    const blob = new Blob([new Uint8Array([1])], { type: "audio/webm" });
    await expect(transcribeAudio(blob, "x.webm", { baseUrl: "http://x" })).rejects.toMatchObject({
      status: 503,
    });
  });

  it("surfaces non-2xx errors as ApiError instances", async () => {
    global.fetch = vi.fn(
      async () => new Response("bad", { status: 502, statusText: "Bad Gateway" }),
    ) as unknown as typeof fetch;

    const blob = new Blob([new Uint8Array([1])], { type: "audio/webm" });
    await expect(transcribeAudio(blob, "x.webm", { baseUrl: "http://x" })).rejects.toBeInstanceOf(
      ApiError,
    );
  });
});
