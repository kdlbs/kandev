import { act, renderHook } from "@testing-library/react";
import { beforeEach, expect, it, vi } from "vitest";
const mocks = vi.hoisted(() => ({ request: vi.fn(), build: vi.fn() }));
vi.mock("@/lib/ws/connection", () => ({ getWebSocketClient: () => ({}) }));
vi.mock("@/lib/ws/workspace-files", () => ({ requestFileContent: mocks.request }));
vi.mock("./file-editor-state", () => ({ buildFileEditorState: mocks.build }));
import { usePreviewAsText } from "./use-preview-as-text";
beforeEach(() => vi.clearAllMocks());
it("resets pending and failed text reads when the selected file changes", async () => {
  let reject: (error: Error) => void = () => {};
  mocks.request.mockReturnValueOnce(
    new Promise((_, no) => {
      reject = no;
    }),
  );
  const hook = renderHook(({ path }) => usePreviewAsText(path, undefined, "session"), {
    initialProps: { path: "one.stl" },
  });
  let pending: Promise<void>;
  act(() => {
    pending = hook.result.current.load();
  });
  expect(hook.result.current.loading).toBe(true);
  hook.rerender({ path: "two.stl" });
  expect(hook.result.current.loading).toBe(false);
  await act(async () => {
    reject(Error());
    await pending;
  });
  expect(hook.result.current.failed).toBe(false);
  mocks.request.mockRejectedValueOnce(Error());
  await act(async () => {
    await hook.result.current.load();
  });
  expect(hook.result.current.failed).toBe(true);
  hook.rerender({ path: "three.stl" });
  expect(hook.result.current.failed).toBe(false);
});
it("discards a pending text read when returning to a viewer", async () => {
  let resolve!: (value: unknown) => void;
  mocks.request.mockReturnValueOnce(
    new Promise((yes) => {
      resolve = yes;
    }),
  );
  mocks.build.mockResolvedValueOnce({ content: "late text" });
  const hook = renderHook(() => usePreviewAsText("part.stl", undefined, "session"));
  let pending!: Promise<void>;
  act(() => {
    pending = hook.result.current.load();
  });
  act(() => {
    hook.result.current.clear();
  });
  await act(async () => {
    resolve({ content: "late text" });
    await pending;
  });
  expect(hook.result.current.file).toBeNull();
  expect(hook.result.current.loading).toBe(false);
});
