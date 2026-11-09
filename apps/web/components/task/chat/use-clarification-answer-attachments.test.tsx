import { act, cleanup, render, screen, waitFor, fireEvent } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { Message } from "@/lib/types/http";
import {
  useClarificationAnswerAttachments,
  type ClarificationAnswerAttachmentController,
} from "./use-clarification-answer-attachments";
import type { FileAttachment } from "./file-attachment";

const API = vi.hoisted(() => ({
  upload: vi.fn(),
  remove: vi.fn(),
  fetchTask: vi.fn(),
  processFile: vi.fn(),
}));

vi.mock("@/lib/api/domains/attachment-api", () => ({
  uploadAttachment: (...args: unknown[]) => API.upload(...args),
  deleteAttachment: (...args: unknown[]) => API.remove(...args),
}));

vi.mock("@/lib/api/domains/kanban-api", () => ({
  fetchTask: (...args: unknown[]) => API.fetchTask(...args),
}));

vi.mock("./file-attachment", async () => {
  const actual = await vi.importActual<typeof import("./file-attachment")>("./file-attachment");
  return { ...actual, processFile: (...args: unknown[]) => API.processFile(...args) };
});

let bundleCounter = 0;

function message(
  overrides: Record<string, unknown> = {},
  pendingId = `pending-${bundleCounter}`,
): Message {
  return {
    id: "m1",
    session_id: "session-1",
    task_id: "task-1",
    metadata: {
      pending_id: pendingId,
      question_id: "q1",
      question: { id: "q1", options: [] },
      ...overrides,
    },
  } as unknown as Message;
}

function processedFile(name: string, type = "image/png"): FileAttachment {
  return {
    id: name,
    file: new File([new Uint8Array([1, 2, 3])], name, { type }),
    mimeType: type,
    fileName: name,
    size: 3,
    isImage: type.startsWith("image/"),
    deliveryMode: "path",
    uploadStatus: "pending",
  };
}

let controller: ClarificationAnswerAttachmentController | null = null;

function Harness({
  messages,
  pendingId,
  id,
}: {
  messages: Message[];
  pendingId: string;
  id?: string;
}) {
  const value = useClarificationAnswerAttachments({
    messages,
    questionIds: ["q1"],
    pendingId,
  });
  controller = value;
  return (
    <div data-testid={id}>
      <output data-testid={`total${id ? `-${id}` : ""}`}>{value.totalCount}</output>
      <output data-testid={`failed${id ? `-${id}` : ""}`}>{value.failedCount}</output>
      <output data-testid={`ready${id ? `-${id}` : ""}`}>{value.allReady ? "yes" : "no"}</output>
      <output data-testid={`attachments${id ? `-${id}` : ""}`}>
        {JSON.stringify(value.readyAttachments("q1"))}
      </output>
      <button
        data-testid={`add${id ? `-${id}` : ""}`}
        onClick={() =>
          value.addFiles("q1", [new File([new Uint8Array([1])], "a.png", { type: "image/png" })])
        }
      />
    </div>
  );
}

function renderHook(messageValue = message()) {
  return render(<Harness messages={[messageValue]} pendingId={`pending-${bundleCounter}`} />);
}

function deferred<T>() {
  let resolve!: (value: T) => void;
  let reject!: (error: Error) => void;
  const promise = new Promise<T>((resolvePromise, rejectPromise) => {
    resolve = resolvePromise;
    reject = rejectPromise;
  });
  return { promise, resolve, reject };
}

beforeEach(() => {
  bundleCounter += 1;
  API.upload.mockReset();
  API.remove.mockReset();
  API.fetchTask.mockReset();
  API.processFile.mockReset();
  API.remove.mockResolvedValue(undefined);
  API.fetchTask.mockResolvedValue({ workspace_id: "workspace-1" });
  API.upload.mockResolvedValue({
    attachment_id: "att-1",
    name: "a.png",
    mime_type: "image/png",
    kind: "image",
    delivery_mode: "path",
    size_bytes: 3,
  });
  API.processFile.mockImplementation(async () => processedFile("a.png"));
});

afterEach(() => {
  cleanup();
  controller = null;
});

describe("useClarificationAnswerAttachments", () => {
  it("uploads picker files with path delivery and exposes a ready descriptor", async () => {
    renderHook();
    fireEvent.click(screen.getByTestId("add"));

    await waitFor(() => expect(screen.getByTestId("ready").textContent).toBe("yes"));
    expect(API.fetchTask).toHaveBeenCalledWith("task-1", { cache: "no-store" });
    expect(API.upload).toHaveBeenCalledWith(
      expect.any(File),
      expect.objectContaining({ workspaceId: "workspace-1", deliveryMode: "path" }),
    );
    const attachments = JSON.parse(screen.getByTestId("attachments").textContent ?? "[]");
    expect(attachments).toEqual([
      {
        attachment_id: "att-1",
        type: "image",
        name: "a.png",
        mime_type: "image/png",
        size_bytes: 3,
        delivery_mode: "path",
      },
    ]);
  });

  it("blocks readiness while uploading and after a failed upload, then recovers on retry", async () => {
    let rejectUpload: ((error: Error) => void) | undefined;
    API.upload.mockImplementationOnce(
      () =>
        new Promise((_, reject) => {
          rejectUpload = reject;
        }),
    );
    renderHook();
    fireEvent.click(screen.getByTestId("add"));
    await waitFor(() => expect(screen.getByTestId("total").textContent).toBe("1"));
    expect(screen.getByTestId("ready").textContent).toBe("no");

    await act(async () => rejectUpload?.(new Error("upload failed")));
    await waitFor(() => expect(screen.getByTestId("failed").textContent).toBe("1"));
    expect(screen.getByTestId("ready").textContent).toBe("no");

    act(() => {
      const files = controller?.filesFor("q1") ?? [];
      controller?.retry("q1", files[0].id);
    });
    await waitFor(() => expect(screen.getByTestId("ready").textContent).toBe("yes"));
    expect(screen.getByTestId("failed").textContent).toBe("0");
  });

  it("restores server-retained files without uploading them again", async () => {
    const retained = message({
      attachments: [
        {
          attachment_id: "att-retained",
          type: "resource",
          name: "log.txt",
          mime_type: "text/plain",
          size_bytes: 9,
          delivery_mode: "path",
        },
      ],
    });
    renderHook(retained);

    await waitFor(() => expect(screen.getByTestId("ready").textContent).toBe("yes"));
    expect(API.upload).not.toHaveBeenCalled();
    const attachments = JSON.parse(screen.getByTestId("attachments").textContent ?? "[]");
    expect(attachments[0]).toMatchObject({
      attachment_id: "att-retained",
      name: "log.txt",
      delivery_mode: "path",
    });
  });
});

describe("useClarificationAnswerAttachments concurrency and remounts", () => {
  it("reserves pending state before asynchronous file processing completes", async () => {
    const processing = deferred<FileAttachment>();
    API.processFile.mockImplementationOnce(() => processing.promise);
    renderHook();
    fireEvent.click(screen.getByTestId("add"));

    await waitFor(() => expect(screen.getByTestId("total").textContent).toBe("1"));
    expect(screen.getByTestId("ready").textContent).toBe("no");

    await act(async () => processing.resolve(processedFile("a.png")));
    await waitFor(() => expect(screen.getByTestId("ready").textContent).toBe("yes"));
  });

  it("does not place a file processed for a replaced bundle into the new draft", async () => {
    const processing = deferred<FileAttachment>();
    API.processFile.mockImplementationOnce(() => processing.promise);
    const first = message({}, "pending-source-a");
    const view = render(<Harness messages={[first]} pendingId="pending-source-a" />);
    fireEvent.click(screen.getByTestId("add"));
    await waitFor(() => expect(screen.getByTestId("total").textContent).toBe("1"));

    const second = message({}, "pending-source-b");
    view.rerender(<Harness messages={[second]} pendingId="pending-source-b" />);
    await waitFor(() => expect(screen.getByTestId("total").textContent).toBe("0"));

    await act(async () => processing.resolve(processedFile("a.png")));
    await new Promise((resolve) => setTimeout(resolve, 0));
    expect(screen.getByTestId("total").textContent).toBe("0");
    expect(screen.getByTestId("ready").textContent).toBe("yes");
  });
});

describe("useClarificationAnswerAttachments source isolation and limits", () => {
  it("does not publish a replaced task's workspace resolution into the new draft", async () => {
    const workspace = deferred<{ workspace_id: string }>();
    API.fetchTask.mockImplementationOnce(() => workspace.promise);
    const first = message({}, "pending-workspace-a");
    const view = render(<Harness messages={[first]} pendingId="pending-workspace-a" />);
    fireEvent.click(screen.getByTestId("add"));
    await waitFor(() => expect(screen.getByTestId("total").textContent).toBe("1"));

    const second = message({}, "pending-workspace-b");
    view.rerender(<Harness messages={[second]} pendingId="pending-workspace-b" />);
    await waitFor(() => expect(screen.getByTestId("total").textContent).toBe("0"));

    await act(async () => workspace.resolve({ workspace_id: "workspace-old" }));
    await new Promise((resolve) => setTimeout(resolve, 0));
    expect(screen.getByTestId("total").textContent).toBe("0");
  });

  it("shares upload completion with every live host of the same bundle", async () => {
    const uploading = deferred<Awaited<ReturnType<typeof API.upload>>>();
    API.upload.mockImplementationOnce(() => uploading.promise);
    const value = message({}, "pending-two-hosts");
    render(
      <>
        <Harness messages={[value]} pendingId="pending-two-hosts" id="host-a" />
        <Harness messages={[value]} pendingId="pending-two-hosts" id="host-b" />
      </>,
    );
    fireEvent.click(screen.getByTestId("add-host-a"));
    await waitFor(() => expect(screen.getByTestId("total-host-b").textContent).toBe("1"));
    expect(screen.getByTestId("ready-host-b").textContent).toBe("no");

    await act(async () =>
      uploading.resolve({
        attachment_id: "att-shared",
        name: "a.png",
        mime_type: "image/png",
        kind: "image",
        delivery_mode: "path",
        size_bytes: 3,
      }),
    );
    await waitFor(() => expect(screen.getByTestId("ready-host-a").textContent).toBe("yes"));
    await waitFor(() => expect(screen.getByTestId("ready-host-b").textContent).toBe("yes"));
    expect(API.upload).toHaveBeenCalledTimes(1);
  });

  it("rejects overlapping additions that exceed the bundle limit", async () => {
    const pending = [
      deferred<FileAttachment>(),
      deferred<FileAttachment>(),
      deferred<FileAttachment>(),
      deferred<FileAttachment>(),
      deferred<FileAttachment>(),
      deferred<FileAttachment>(),
    ];
    let index = 0;
    API.processFile.mockImplementation(() => pending[index++].promise);
    renderHook();
    act(() =>
      controller?.addFiles(
        "q1",
        Array.from(
          { length: 6 },
          (_, value) => new File([`${value}`], `${value}.png`, { type: "image/png" }),
        ),
      ),
    );
    await waitFor(() => expect(screen.getByTestId("total").textContent).toBe("6"));
    act(() =>
      controller?.addFiles(
        "q1",
        Array.from(
          { length: 5 },
          (_, value) => new File([`${value}`], `extra-${value}.png`, { type: "image/png" }),
        ),
      ),
    );
    expect(screen.getByTestId("total").textContent).toBe("6");
    await act(async () => {
      for (const item of pending) item.resolve(processedFile("a.png"));
    });
    await waitFor(() => expect(screen.getByTestId("ready").textContent).toBe("yes"));
    expect(Number(screen.getByTestId("total").textContent)).toBeLessThanOrEqual(10);
  });
});

describe("useClarificationAnswerAttachments remounts and abandoned uploads", () => {
  it("becomes ready after a remount finishes the shared pending upload", async () => {
    const uploading = deferred<Awaited<ReturnType<typeof API.upload>>>();
    API.upload.mockImplementationOnce(() => uploading.promise);
    const value = message({}, "pending-remount");
    const first = render(<Harness messages={[value]} pendingId="pending-remount" />);
    fireEvent.click(screen.getByTestId("add"));
    await waitFor(() => expect(screen.getByTestId("total").textContent).toBe("1"));
    expect(screen.getByTestId("ready").textContent).toBe("no");
    first.unmount();

    render(<Harness messages={[value]} pendingId="pending-remount" />);
    await waitFor(() => expect(screen.getByTestId("total").textContent).toBe("1"));
    expect(screen.getByTestId("ready").textContent).toBe("no");

    await act(async () =>
      uploading.resolve({
        attachment_id: "att-remount",
        name: "a.png",
        mime_type: "image/png",
        kind: "image",
        delivery_mode: "path",
        size_bytes: 3,
      }),
    );
    await waitFor(() => expect(screen.getByTestId("ready").textContent).toBe("yes"));
    expect(JSON.parse(screen.getByTestId("attachments").textContent ?? "[]")[0]).toMatchObject({
      attachment_id: "att-remount",
    });
  });

  it("deletes a staged upload that finishes after its file was removed", async () => {
    const uploading = deferred<Awaited<ReturnType<typeof API.upload>>>();
    API.upload.mockImplementationOnce(() => uploading.promise);
    renderHook();
    fireEvent.click(screen.getByTestId("add"));
    await waitFor(() => {
      expect(screen.getByTestId("total").textContent).toBe("1");
      expect(API.upload).toHaveBeenCalledTimes(1);
    });
    const files = controller?.filesFor("q1") ?? [];
    const removalsBefore = API.remove.mock.calls.length;
    act(() => controller?.remove("q1", files[0].id));
    expect(API.remove.mock.calls.length).toBe(removalsBefore);

    await act(async () =>
      uploading.resolve({
        attachment_id: "att-abandoned",
        name: "a.png",
        mime_type: "image/png",
        kind: "image",
        delivery_mode: "path",
        size_bytes: 3,
      }),
    );
    await waitFor(() => expect(API.remove).toHaveBeenCalledWith("att-abandoned"));
  });

  it("releases an abandoned file-backed draft after the idle window", async () => {
    const view = renderHook();
    fireEvent.click(screen.getByTestId("add"));
    await waitFor(() => expect(screen.getByTestId("ready").textContent).toBe("yes"));

    vi.useFakeTimers();
    try {
      view.unmount();
      act(() => vi.advanceTimersByTime(15 * 60 * 1000));
      expect(API.remove).toHaveBeenCalledWith("att-1");
    } finally {
      vi.useRealTimers();
    }
  });
});

describe("useClarificationAnswerAttachments retained drafts", () => {
  it("does not re-import a retained file after the user removes it", async () => {
    const retained = message(
      {
        attachments: [
          {
            attachment_id: "att-retained-removed",
            type: "resource",
            name: "evidence.txt",
            mime_type: "text/plain",
            size_bytes: 5,
            delivery_mode: "path",
          },
        ],
      },
      "pending-retained-removed",
    );
    const view = render(<Harness messages={[retained]} pendingId="pending-retained-removed" />);
    await waitFor(() => expect(screen.getByTestId("total").textContent).toBe("1"));
    const files = controller?.filesFor("q1") ?? [];
    act(() => controller?.remove("q1", files[0].id));
    await waitFor(() => expect(screen.getByTestId("total").textContent).toBe("0"));

    view.rerender(<Harness messages={[retained]} pendingId="pending-retained-removed" />);
    await new Promise((resolve) => setTimeout(resolve, 0));
    expect(screen.getByTestId("total").textContent).toBe("0");
    expect(screen.getByTestId("ready").textContent).toBe("yes");
  });

  it("finishes an upload that was still waiting for workspace after unmount", async () => {
    const workspace = deferred<{ workspace_id: string }>();
    const uploading = deferred<Awaited<ReturnType<typeof API.upload>>>();
    API.fetchTask.mockImplementationOnce(() => workspace.promise);
    API.upload.mockImplementationOnce(() => uploading.promise);
    const value = message({}, "pending-upload-unmounted");
    const first = render(<Harness messages={[value]} pendingId="pending-upload-unmounted" />);
    fireEvent.click(screen.getByTestId("add"));
    await waitFor(() => expect(screen.getByTestId("total").textContent).toBe("1"));
    first.unmount();

    await act(async () => workspace.resolve({ workspace_id: "workspace-1" }));
    await waitFor(() => expect(API.upload).toHaveBeenCalledTimes(1));
    render(<Harness messages={[value]} pendingId="pending-upload-unmounted" />);
    expect(screen.getByTestId("ready").textContent).toBe("no");

    await act(async () =>
      uploading.resolve({
        attachment_id: "att-after-unmount",
        name: "a.png",
        mime_type: "image/png",
        kind: "image",
        delivery_mode: "path",
        size_bytes: 3,
      }),
    );
    await waitFor(() => expect(screen.getByTestId("ready").textContent).toBe("yes"));
  });
});

describe("useClarificationAnswerAttachments removal", () => {
  it("removes a staged upload without disturbing ready siblings", async () => {
    renderHook();
    fireEvent.click(screen.getByTestId("add"));
    await waitFor(() => expect(screen.getByTestId("ready").textContent).toBe("yes"));
    const files = controller?.filesFor("q1") ?? [];
    act(() => controller?.remove("q1", files[0].id));
    await waitFor(() => expect(screen.getByTestId("total").textContent).toBe("0"));
    expect(API.remove).toHaveBeenCalledWith("att-1");
  });
});
