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
it("reloads the authoritative revision after an uncertain write and permits retry", async () => {
  const hook = renderHook(() => useFilePreviewPolicy("plugin", "workspace"));
  await waitFor(() => expect(hook.result.current.policy).not.toBeNull());
  api.set.mockRejectedValueOnce(Error("conflict or lost response"));
  api.get.mockResolvedValueOnce({ mode: "deny", revision: 4 });
  await act(async () => {
    await hook.result.current.update("allow");
  });
  expect(hook.result.current.failed).toBe(true);
  expect(hook.result.current.policy).toEqual({ mode: "deny", revision: 4 });
  api.set.mockResolvedValueOnce({ mode: "allow", revision: 5 });
  await act(async () => {
    await hook.result.current.update("allow");
  });
  expect(api.set).toHaveBeenLastCalledWith(
    { pluginId: "plugin", workspaceId: "workspace" },
    "allow",
    4,
  );
  expect(hook.result.current.failed).toBe(false);
});
