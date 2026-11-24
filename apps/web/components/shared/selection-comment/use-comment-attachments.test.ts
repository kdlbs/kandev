import { act, renderHook, waitFor } from "@testing-library/react";
import { expect, it, vi } from "vitest";
import { uploadAttachment, deleteAttachment } from "@/lib/api/domains/attachment-api";
import { useCommentAttachments } from "./use-comment-attachments";

vi.mock("@/lib/api/domains/attachment-api", () => ({
  uploadAttachment: vi.fn(),
  deleteAttachment: vi.fn().mockResolvedValue(undefined),
}));
vi.mock("@/lib/api/domains/kanban-api", () => ({
  fetchTask: vi.fn().mockResolvedValue({ workspace_id: "workspace" }),
}));

it("retains attachment order across retries and discards results removed while uploading", async () => {
  const descriptor = {
    attachment_id: "first" as string,
    name: "file.txt",
    mime_type: "text/plain",
    kind: "resource",
    delivery_mode: "path",
    size_bytes: 1,
  } as const;
  let resolveLate!: (value: typeof descriptor) => void;
  vi.mocked(uploadAttachment)
    .mockRejectedValueOnce(new Error("upload failed"))
    .mockImplementationOnce(() => new Promise((resolve) => (resolveLate = resolve)))
    .mockResolvedValueOnce({ ...descriptor, attachment_id: "third" })
    .mockResolvedValueOnce(descriptor);
  const { result } = renderHook(() => useCommentAttachments({ taskId: "task" }));
  act(() => result.current.addFiles([1, 2, 3].map(() => new File(["a"], "file.txt"))));
  await waitFor(() => expect(result.current.files[0]?.status).toBe("failed"));
  expect(result.current.ready).toBe(false);
  act(() => {
    result.current.remove(result.current.files[1].key);
    result.current.retry(result.current.files[0].key);
    resolveLate({ ...descriptor, attachment_id: "removed" });
  });
  await waitFor(() => expect(result.current.ready).toBe(true));
  expect(result.current.descriptors.map((file) => file.attachment_id)).toEqual(["first", "third"]);
  expect(deleteAttachment).toHaveBeenCalledWith("removed");
});

it("intercepts file paste and leaves text paste to the browser", async () => {
  vi.mocked(uploadAttachment).mockResolvedValueOnce({
    attachment_id: "pasted",
    name: "screen.png",
    mime_type: "image/png",
    kind: "image",
    delivery_mode: "path",
    size_bytes: 1,
  });
  const { result } = renderHook(() => useCommentAttachments({ taskId: "task" }));
  const preventDefault = vi.fn();
  const clipboard = (files: File[], text = "") =>
    ({ files, items: [], getData: () => text }) as unknown as DataTransfer;
  act(() => {
    result.current.handlePaste({ clipboardData: clipboard([], "comment text"), preventDefault });
  });
  expect(preventDefault).not.toHaveBeenCalled();
  act(() => {
    result.current.handlePaste({
      clipboardData: clipboard([new File(["a"], "screen.png", { type: "image/png" })]),
      preventDefault,
    });
  });
  expect(preventDefault).toHaveBeenCalledTimes(1);
  await waitFor(() => expect(result.current.descriptors[0]?.attachment_id).toBe("pasted"));
});
