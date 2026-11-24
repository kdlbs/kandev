import { act, cleanup, renderHook, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, expect, it, vi } from "vitest";
import { usePluginFilePreview } from "./use-plugin-file-preview";
const api = vi.hoisted(() => ({
  prepare: vi.fn(),
  read: vi.fn(),
  cancel: vi.fn(),
  policy: vi.fn(),
}));
vi.mock("@/lib/plugins/file-preview-client", () => ({
  preparePreview: api.prepare,
  readPreview: api.read,
  cancelPreview: api.cancel,
  getPreviewPolicy: api.policy,
}));
const selection = {
  id: "selection",
  path: "/tmp/model.stl",
  size_bytes: 4,
  external: true,
  approval_required: true,
};
const input = {
  path: "/tmp/model.stl",
  sessionId: "session",
  workspaceId: "workspace",
  viewer: {
    pluginId: "plugin",
    id: "stl",
    title: "STL",
    extensions: ["stl"],
    generation: 1,
    Component: () => null,
  },
};
beforeEach(() => {
  api.prepare.mockResolvedValue(selection);
  api.read.mockResolvedValue(new ArrayBuffer(4));
  api.cancel.mockResolvedValue(undefined);
  api.policy.mockResolvedValue({ mode: "ask", revision: 0 });
});
afterEach(() => {
  cleanup();
  vi.clearAllMocks();
});
it("automatically loads workspace files and releases them on unmount", async () => {
  api.prepare.mockResolvedValue({ ...selection, external: false, approval_required: false });
  const { result, unmount } = renderHook(() => usePluginFilePreview(input));
  await waitFor(() => expect(result.current.state.status).toBe("ready"));
  expect(api.prepare).toHaveBeenCalledTimes(1);
  expect(api.read).toHaveBeenCalledTimes(1);
  unmount();
  expect(api.cancel).toHaveBeenCalledWith(
    { pluginId: "plugin", workspaceId: "workspace" },
    "selection",
  );
});
it("automatically checks external files but waits for approval before reading", async () => {
  const { result } = renderHook(() => usePluginFilePreview(input));
  await waitFor(() => expect(result.current.state.status).toBe("approval"));
  expect(api.read).not.toHaveBeenCalled();
  act(() => result.current.approve());
  await waitFor(() => expect(result.current.state.status).toBe("ready"));
  expect(api.read).toHaveBeenCalledTimes(1);
});
it("dismisses external approval without reading or automatically prompting again", async () => {
  const { result, rerender } = renderHook(() => usePluginFilePreview(input));
  await waitFor(() => expect(result.current.state.status).toBe("approval"));
  act(() => result.current.dismissApproval());
  rerender();
  expect(result.current.state.status).toBe("idle");
  expect(api.read).not.toHaveBeenCalled();
  expect(api.prepare).toHaveBeenCalledTimes(1);
  await act(() => result.current.request());
  expect(result.current.state.status).toBe("approval");
});
it("cannot dismiss a running preview through the approval action", async () => {
  api.prepare.mockResolvedValue({ ...selection, approval_required: false });
  const { result } = renderHook(() => usePluginFilePreview(input));
  await waitFor(() => expect(result.current.state.status).toBe("ready"));
  act(() => result.current.dismissApproval());
  expect(result.current.state.status).toBe("ready");
  expect(api.cancel).not.toHaveBeenCalled();
});
it("discards a previous file's bytes after switching files", async () => {
  let complete!: (value: ArrayBuffer) => void;
  api.prepare.mockResolvedValueOnce({ ...selection, approval_required: false });
  api.read.mockReturnValueOnce(
    new Promise((resolve) => {
      complete = resolve;
    }),
  );
  const { result, rerender } = renderHook((props) => usePluginFilePreview(props), {
    initialProps: input,
  });
  await waitFor(() => expect(api.read).toHaveBeenCalledTimes(1));
  const signal = api.read.mock.calls[0][2];
  rerender({ ...input, path: "/tmp/next.stl" });
  await waitFor(() => expect(result.current.state.status).toBe("approval"));
  expect(signal.aborted).toBe(true);
  await act(async () => complete(new ArrayBuffer(4)));
  expect(result.current.state.status).toBe("approval");
  expect(result.current.state.bytes).toBeUndefined();
});
it("does not retry failed reads until the user retries", async () => {
  api.prepare.mockResolvedValue({ ...selection, approval_required: false });
  api.read.mockRejectedValueOnce(new Error());
  const { result, rerender } = renderHook(() => usePluginFilePreview(input));
  await waitFor(() => expect(result.current.state.status).toBe("failed"));
  rerender();
  expect(api.prepare).toHaveBeenCalledTimes(1);
  await act(() => result.current.request());
  expect(result.current.state.status).toBe("ready");
});
it("releases bytes on policy changes without automatically reacquiring them", async () => {
  api.prepare.mockResolvedValue({ ...selection, approval_required: false });
  const { result } = renderHook(() => usePluginFilePreview(input));
  await waitFor(() => expect(result.current.state.status).toBe("ready"));
  act(() => window.dispatchEvent(new Event("kandev-preview-policy-changed")));
  expect(result.current.state.status).toBe("idle");
  expect(result.current.state.bytes).toBeUndefined();
  expect(api.cancel).toHaveBeenCalledTimes(1);
  expect(api.prepare).toHaveBeenCalledTimes(1);
});
it("does not prepare a file after an unmounted policy request resolves", async () => {
  let complete!: (value: { mode: string; revision: number }) => void;
  api.policy.mockReturnValueOnce(
    new Promise((resolve) => {
      complete = resolve;
    }),
  );
  const { unmount } = renderHook(() => usePluginFilePreview(input));
  unmount();
  await act(async () => complete({ mode: "ask", revision: 0 }));
  expect(api.prepare).not.toHaveBeenCalled();
});
