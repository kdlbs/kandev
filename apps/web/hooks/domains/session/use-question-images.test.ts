import { act, renderHook, waitFor } from "@testing-library/react";
import { beforeEach, expect, it, vi } from "vitest";
import { useQuestionImages } from "./use-question-images";

const mocks = vi.hoisted(() => ({ fetch: vi.fn() }));
vi.mock("@/lib/api/domains/question-image-api", () => ({
  fetchQuestionImage: mocks.fetch,
  questionImageErrorCode: () => "unavailable",
}));
const images = [1, 2, 3].map((id) => ({ id: `image${id}`, path: `${id}.png`, alt: `Image ${id}` }));
beforeEach(() => {
  vi.clearAllMocks();
  vi.stubGlobal("URL", { createObjectURL: vi.fn(() => "blob:test"), revokeObjectURL: vi.fn() });
});

it("loads only active images, with two reads, and revokes blobs on owner change", async () => {
  const pending: ((blob: Blob) => void)[] = [];
  mocks.fetch.mockImplementation(() => new Promise<Blob>((resolve) => pending.push(resolve)));
  const { result, rerender, unmount } = renderHook(
    ({ active, owner }) => useQuestionImages("bundle", "question", images, active, owner),
    { initialProps: { active: false, owner: "source-session:user" } },
  );
  expect(mocks.fetch).not.toHaveBeenCalled();
  rerender({ active: true, owner: "source-session:user" });
  await waitFor(() => expect(mocks.fetch).toHaveBeenCalledTimes(2));
  expect(mocks.fetch).toHaveBeenCalledWith("bundle", "question", "image1", expect.any(AbortSignal));
  await act(async () => pending[0](new Blob(["png"], { type: "image/png" })));
  await waitFor(() => expect(mocks.fetch).toHaveBeenCalledTimes(3));
  expect(result.current.states.image1.kind).toBe("loaded");
  const firstSignal = mocks.fetch.mock.calls[0][3] as AbortSignal;
  rerender({ active: true, owner: "other-session:user" });
  expect(firstSignal.aborted).toBe(true);
  expect(URL.revokeObjectURL).toHaveBeenCalledWith("blob:test");
  await act(async () => pending[1](new Blob(["old"], { type: "image/png" })));
  expect(result.current.states.image2?.kind).not.toBe("loaded");
  unmount();
  await act(async () => pending.forEach((resolve) => resolve(new Blob(["cleanup"]))));
});

it("requires explicit retry after a failure", async () => {
  mocks.fetch.mockRejectedValueOnce(new Error("unavailable")).mockResolvedValue(new Blob(["png"]));
  const { result } = renderHook(() =>
    useQuestionImages("bundle", "q", images.slice(0, 1), true, "owner"),
  );
  await waitFor(() => expect(result.current.states.image1.kind).toBe("error"));
  expect(mocks.fetch).toHaveBeenCalledTimes(1);
  act(() => result.current.retry());
  await waitFor(() => expect(result.current.states.image1.kind).toBe("loaded"));
});

it("shares two reads across question and choice hooks and cancels queued choices", async () => {
  const pending: (() => void)[] = [];
  mocks.fetch.mockImplementation(
    (_pending, _question, _image, signal: AbortSignal) =>
      new Promise<Blob>((resolve, reject) => {
        const abort = () => reject(signal.reason);
        signal.addEventListener("abort", abort, { once: true });
        pending.push(() => {
          signal.removeEventListener("abort", abort);
          resolve(new Blob(["png"], { type: "image/png" }));
        });
      }),
  );
  const question = renderHook(() =>
    useQuestionImages("shared", "q", images.slice(0, 2), true, "owner"),
  );
  const choice = renderHook(() => useQuestionImages("shared", "q", images.slice(2), true, "owner"));
  const nextChoice = renderHook(() =>
    useQuestionImages("shared", "q", [{ ...images[2], id: "image4" }], true, "owner"),
  );
  await waitFor(() => expect(mocks.fetch).toHaveBeenCalledTimes(2));
  choice.unmount();
  await act(async () => pending[0]());
  await waitFor(() => expect(mocks.fetch).toHaveBeenCalledTimes(3));
  expect(mocks.fetch.mock.calls[2][2]).toBe("image4");
  await act(async () => {
    pending[1]();
    pending[2]();
  });
  question.unmount();
  nextChoice.unmount();
});
