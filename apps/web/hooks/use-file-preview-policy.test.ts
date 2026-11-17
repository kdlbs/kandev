import { act, renderHook, waitFor } from "@testing-library/react";
import { beforeEach, expect, it, vi } from "vitest";
import { useFilePreviewPolicy } from "./use-file-preview-policy";
const api = vi.hoisted(() => ({ get: vi.fn(), set: vi.fn() }));
vi.mock("@/lib/plugins/file-preview-client", () => ({
  getPreviewPolicy: api.get,
  setPreviewPolicy: api.set,
}));
beforeEach(() => {
  vi.clearAllMocks();
  api.get.mockResolvedValue({ mode: "ask", revision: 0 });
});
it("ignores a policy write response after the selected workspace changes", async () => {
  let finish!: (value: unknown) => void;
  api.set.mockImplementation(
    () =>
      new Promise((resolve) => {
        finish = resolve;
      }),
  );
  const hook = renderHook(({ workspace }) => useFilePreviewPolicy("plugin", workspace), {
    initialProps: { workspace: "one" },
  });
  await waitFor(() => expect(hook.result.current.policy?.mode).toBe("ask"));
  let pending!: Promise<void>;
  act(() => {
    pending = hook.result.current.update("allow");
  });
  hook.rerender({ workspace: "two" });
  await waitFor(() => expect(hook.result.current.policy?.mode).toBe("ask"));
  await act(async () => {
    finish({ mode: "allow", revision: 1 });
    await pending;
  });
  expect(hook.result.current.policy).toEqual({ mode: "ask", revision: 0 });
  expect(hook.result.current.saving).toBe(false);
});
