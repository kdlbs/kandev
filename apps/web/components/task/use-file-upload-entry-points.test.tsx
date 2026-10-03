import { flushSync } from "react-dom";
import { act, cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { useFileUploadEntryPoints } from "./use-file-upload-entry-points";

const FIRST_PATH = "fixtures/a.txt";
const { preflight, upload, toast } = vi.hoisted(() => ({
  preflight: vi.fn(),
  upload: vi.fn(),
  toast: vi.fn(),
}));
vi.mock("@/lib/api/domains/workspace-file-api", () => ({
  preflightWorkspaceUpload: preflight,
  uploadWorkspaceFile: upload,
}));
vi.mock("@/components/toast-provider", () => ({ useToast: () => ({ toast }) }));

function Owner({ sessionId }: { sessionId: string }) {
  const { elements, openPicker } = useFileUploadEntryPoints(sessionId);
  return (
    <>
      <button onClick={() => openPicker("files", "fixtures")}>Pick files</button>
      {elements}
    </>
  );
}
function pickFiles(names = ["a.txt", "b.txt"]) {
  fireEvent.click(screen.getByRole("button", { name: "Pick files" }));
  fireEvent.change(screen.getByTestId("files-upload-input"), {
    target: { files: names.map((name) => new File(["bytes"], name)) },
  });
}
function deferredUpload() {
  let resolve!: (result: { path: string; size_bytes: number }) => void;
  let reject!: (error: Error) => void;
  const promise = new Promise<{ path: string; size_bytes: number }>((yes, no) => {
    resolve = yes;
    reject = no;
  });
  upload.mockReturnValueOnce(promise);
  return { promise, resolve, reject };
}
beforeEach(() => {
  vi.clearAllMocks();
  preflight.mockReset().mockResolvedValue([]);
  upload.mockReset().mockImplementation(async ({ relativePath }) => ({
    path: `fixtures/${relativePath}`,
    size_bytes: 5,
  }));
});
afterEach(cleanup);

// @covers AC-UI-WORKSPACE-FILE-TRANSFER-001.5, AC-UI-WORKSPACE-FILE-TRANSFER-003.7
// @covers AC-UI-WORKSPACE-FILE-TRANSFER-003.9, AC-UI-WORKSPACE-FILE-TRANSFER-004.5
describe("workspace upload entry-point report routing", () => {
  it.each([
    ["unmount", false, false],
    ["unmount", true, false],
    ["unmount", false, true],
    ["session", false, false],
    ["session", true, false],
    ["session", true, true],
  ])(
    "suppresses retired reports after %s (conflict %s, failure %s)",
    async (retire, conflict, failure) => {
      if (conflict) preflight.mockResolvedValue([{ path: FIRST_PATH, is_dir: false }]);
      const transport = deferredUpload();
      const owner = render(<Owner sessionId="sess-1" />);
      pickFiles();
      if (conflict) {
        await screen.findByRole("dialog");
        expect(upload).not.toHaveBeenCalled();
        fireEvent.click(screen.getByRole("button", { name: "Upload" }));
      }
      await waitFor(() => expect(upload).toHaveBeenCalledTimes(1));
      if (retire === "unmount") owner.unmount();
      else flushSync(() => owner.rerender(<Owner sessionId="sess-2" />));
      await act(async () => {
        if (failure) transport.reject(new Error("upload failed"));
        else transport.resolve({ path: "fixtures/a-1.txt", size_bytes: 5 });
        await transport.promise.catch(() => undefined);
      });
      expect(upload).toHaveBeenCalledTimes(1);
      expect(toast).not.toHaveBeenCalled();
      expect(screen.queryByRole("dialog")).toBeNull();
      if (retire === "session") {
        preflight.mockResolvedValue([]);
        pickFiles(["current.txt"]);
        await waitFor(() => expect(toast).toHaveBeenCalledTimes(1));
        expect(toast).toHaveBeenLastCalledWith(
          expect.objectContaining({ description: "Uploaded to fixtures/current.txt" }),
        );
      }
    },
  );
  it.each(["unmount", "session"])(
    "suppresses a completed result retired before caller reporting by %s",
    async (retire) => {
      const transport = deferredUpload();
      const owner = render(<Owner sessionId="sess-1" />);
      pickFiles(["a.txt"]);
      await waitFor(() => expect(upload).toHaveBeenCalledTimes(1));
      // The transport resumes the hook first. This microtask retires the owner
      // after uploadFiles completes, before handlePicked resumes to report.
      const retired = transport.promise.then(
        () =>
          new Promise<void>((resolve) => {
            queueMicrotask(() => {
              if (retire === "unmount") owner.unmount();
              else flushSync(() => owner.rerender(<Owner sessionId="sess-2" />));
              resolve();
            });
          }),
      );
      await act(async () => {
        transport.resolve({ path: FIRST_PATH, size_bytes: 5 });
        await retired;
      });
      expect(upload).toHaveBeenCalledTimes(1);
      expect(toast).not.toHaveBeenCalled();
    },
  );
});

describe("workspace upload current reports", () => {
  it("disposes a parked conflict dialog without reporting or writing", async () => {
    preflight.mockResolvedValue([{ path: FIRST_PATH, is_dir: false }]);
    const owner = render(<Owner sessionId="sess-1" />);
    pickFiles();
    await screen.findByRole("dialog");
    await act(async () => owner.unmount());
    expect(screen.queryByRole("dialog")).toBeNull();
    expect(upload).not.toHaveBeenCalled();
    expect(toast).not.toHaveBeenCalled();
  });
  it("reports current success with the server path after conflict resolution", async () => {
    preflight.mockResolvedValue([{ path: FIRST_PATH, is_dir: false }]);
    upload.mockResolvedValue({ path: "fixtures/a-1.txt", size_bytes: 5 });
    render(<Owner sessionId="sess-1" />);
    pickFiles(["a.txt"]);
    await screen.findByRole("dialog");
    fireEvent.click(screen.getByRole("button", { name: "Upload" }));
    await waitFor(() => expect(toast).toHaveBeenCalledTimes(1));
    expect(upload.mock.calls[0][0]).toMatchObject({ resolution: "keep_both" });
    expect(toast).toHaveBeenCalledWith(
      expect.objectContaining({ description: "Uploaded to fixtures/a-1.txt" }),
    );
  });
  it("reports current partial failure without discarding success", async () => {
    upload.mockRejectedValueOnce(new Error("upload failed"));
    render(<Owner sessionId="sess-1" />);
    pickFiles();
    await waitFor(() => expect(toast).toHaveBeenCalledTimes(2));
    expect(toast.mock.calls.map(([value]) => value.variant)).toEqual([undefined, "error"]);
    expect(toast.mock.calls[0][0].description).toBe("Uploaded to fixtures/b.txt");
  });
  it("cancels through the real dialog without writing or reporting", async () => {
    preflight.mockResolvedValue([{ path: FIRST_PATH, is_dir: false }]);
    render(<Owner sessionId="sess-1" />);
    pickFiles();
    await screen.findByRole("dialog");
    await act(async () => fireEvent.click(screen.getByRole("button", { name: "Cancel" })));
    expect(screen.queryByRole("dialog")).toBeNull();
    expect(upload).not.toHaveBeenCalled();
    expect(toast).not.toHaveBeenCalled();
  });
});
