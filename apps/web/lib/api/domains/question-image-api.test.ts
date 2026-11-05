import { expect, it, vi } from "vitest";
import { fetchQuestionImage } from "./question-image-api";
const mocks = vi.hoisted(() => ({ fetchBlob: vi.fn() }));
vi.mock("../client", () => ({ fetchBlob: mocks.fetchBlob, ApiError: class extends Error {} }));
it("addresses saved identities without accepting a filesystem path or redirects", async () => {
  mocks.fetchBlob.mockResolvedValue(new Blob(["png"], { type: "image/png" }));
  const signal = new AbortController().signal;
  await fetchQuestionImage("bundle", "q/with space", "screen", signal);
  expect(mocks.fetchBlob).toHaveBeenCalledWith(
    "/api/v1/clarification/bundle/images/screen?question_id=q%2Fwith%20space",
    { cache: "no-store", init: { signal, redirect: "error" } },
  );
});
it("rejects a successful response with an unsafe content type", async () => {
  mocks.fetchBlob.mockResolvedValue(new Blob(["<svg/>"], { type: "image/svg+xml" }));
  await expect(
    fetchQuestionImage("bundle", "q", "image", new AbortController().signal),
  ).rejects.toThrow();
});
