import { expect, it } from "vitest";
import { consumePreviewResponse } from "./file-preview-client";
it("requires exact size and completion marker", async () => {
  const signal = new AbortController().signal;
  const data = await consumePreviewResponse(new Response(new Uint8Array([1, 2, 0])), 2, signal);
  expect([...new Uint8Array(data)]).toEqual([1, 2]);
  for (const bytes of [
    [1, 2],
    [1, 2, 9],
    [1, 2, 0, 0],
  ]) {
    await expect(
      consumePreviewResponse(new Response(new Uint8Array(bytes)), 2, signal),
    ).rejects.toThrow();
  }
});
it("refuses cancelled and over-limit responses", async () => {
  const abort = new AbortController();
  abort.abort();
  await expect(
    consumePreviewResponse(new Response(new Uint8Array([0])), 0, abort.signal),
  ).rejects.toThrow();
  await expect(
    consumePreviewResponse(new Response(), (64 << 20) + 1, new AbortController().signal),
  ).rejects.toThrow();
});

it("does not allocate from an unsafe advertised length", async () => {
  await expect(
    consumePreviewResponse(new Response(), Number.NaN, new AbortController().signal),
  ).rejects.toThrow();
});
