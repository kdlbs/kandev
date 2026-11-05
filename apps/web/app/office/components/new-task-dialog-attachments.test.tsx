import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { act, cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { TooltipProvider } from "@kandev/ui/tooltip";
import { StateProvider, useAppStoreApi } from "@/components/state-provider";
import { ToastProvider } from "@/components/toast-provider";
import { NewTaskDialog } from "./new-task-dialog";

const { createTask, uploadAttachment, deleteAttachment } = vi.hoisted(() => ({
  createTask: vi.fn().mockResolvedValue({ id: "task-created" }),
  uploadAttachment: vi.fn(),
  deleteAttachment: vi.fn().mockResolvedValue(undefined),
}));

vi.mock("@/lib/api/domains/kanban-api", () => ({ createTask }));
vi.mock("@/lib/api/domains/attachment-api", async (importOriginal) => {
  const actual = await importOriginal<typeof import("@/lib/api/domains/attachment-api")>();
  return { ...actual, uploadAttachment, deleteAttachment };
});

const WORKSPACE_ID = "workspace-office-attachments";
const CREATE_BUTTON = "new-task-create-button";
const TITLE_PLACEHOLDER = "Task title";
const RETRY_BUTTON = "attachment-upload-retry";

function Harness() {
  const store = useAppStoreApi();
  store.getState().setActiveWorkspace(WORKSPACE_ID);
  return <NewTaskDialog open onOpenChange={vi.fn()} defaultProjectId="project-1" />;
}

function renderDialog() {
  return render(
    <StateProvider>
      <ToastProvider>
        <TooltipProvider>
          <Harness />
        </TooltipProvider>
      </ToastProvider>
    </StateProvider>,
  );
}

function uploaded(id = "attachment-1", size = 5) {
  return {
    attachment_id: id,
    name: "notes.txt",
    mime_type: "text/plain",
    kind: "resource" as const,
    delivery_mode: "path" as const,
    size_bytes: size,
    expires_at: new Date(Date.now() + 60_000).toISOString(),
  };
}

function selectFile(name = "notes.txt", contents = "hello") {
  const input = document.querySelector('input[type="file"]');
  if (!(input instanceof HTMLInputElement)) throw new Error("file input not found");
  const file = new File([contents], name, { type: "text/plain" });
  Object.defineProperty(input, "files", { value: [file], configurable: true });
  fireEvent.change(input);
  return file;
}

beforeEach(() => {
  localStorage.clear();
  createTask.mockClear();
  uploadAttachment.mockReset();
  deleteAttachment.mockClear();
});

afterEach(() => cleanup());

// eslint-disable-next-line max-lines-per-function -- attachment cases share the same provider and mock set.
describe("Office New Task attachments", () => {
  it("opens the file picker and submits a ready attachment", async () => {
    uploadAttachment.mockResolvedValue(uploaded());
    renderDialog();
    fireEvent.change(screen.getByPlaceholderText(TITLE_PLACEHOLDER), {
      target: { value: "Ship the attachment flow" },
    });

    selectFile();
    await waitFor(() => expect(uploadAttachment).toHaveBeenCalledTimes(1));
    await waitFor(() => expect(screen.getByText("notes.txt")).toBeTruthy());
    expect((screen.getByTestId(CREATE_BUTTON) as HTMLButtonElement).disabled).toBe(false);

    fireEvent.click(screen.getByTestId(CREATE_BUTTON));
    await waitFor(() => expect(createTask).toHaveBeenCalledTimes(1));
    expect(createTask.mock.calls[0][0].attachments).toEqual([
      expect.objectContaining({
        attachment_id: "attachment-1",
        name: "notes.txt",
        type: "resource",
        delivery_mode: "path",
      }),
    ]);
    expect(deleteAttachment).not.toHaveBeenCalled();
  });

  it("keeps Create disabled while an upload is pending", async () => {
    let resolveUpload: (value: ReturnType<typeof uploaded>) => void = () => undefined;
    uploadAttachment.mockReturnValue(
      new Promise((resolve) => {
        resolveUpload = resolve;
      }),
    );
    renderDialog();
    fireEvent.change(screen.getByPlaceholderText(TITLE_PLACEHOLDER), {
      target: { value: "Pending attachment" },
    });

    selectFile();
    await waitFor(() => expect(uploadAttachment).toHaveBeenCalledTimes(1));
    expect((screen.getByTestId(CREATE_BUTTON) as HTMLButtonElement).disabled).toBe(true);
    fireEvent.click(screen.getByTestId(CREATE_BUTTON));
    expect(createTask).not.toHaveBeenCalled();

    await act(async () => {
      resolveUpload(uploaded());
    });
    await waitFor(() =>
      expect((screen.getByTestId(CREATE_BUTTON) as HTMLButtonElement).disabled).toBe(false),
    );
  });

  it("keeps Create disabled after a failed upload and retries", async () => {
    uploadAttachment
      .mockRejectedValueOnce(new Error("upload failed"))
      .mockResolvedValueOnce(uploaded("attachment-retry"));
    renderDialog();
    fireEvent.change(screen.getByPlaceholderText(TITLE_PLACEHOLDER), {
      target: { value: "Failed attachment" },
    });

    selectFile();
    await waitFor(() => expect(screen.getByText("notes.txt")).toBeTruthy());
    expect((screen.getByTestId(CREATE_BUTTON) as HTMLButtonElement).disabled).toBe(true);
    fireEvent.click(screen.getByTestId(CREATE_BUTTON));
    expect(createTask).not.toHaveBeenCalled();

    fireEvent.click(screen.getByText("notes.txt"));
    await waitFor(() => expect(screen.getByTestId(RETRY_BUTTON)).toBeTruthy());
    fireEvent.click(screen.getByTestId(RETRY_BUTTON));
    await waitFor(() =>
      expect((screen.getByTestId(CREATE_BUTTON) as HTMLButtonElement).disabled).toBe(false),
    );
    await waitFor(() => expect(uploadAttachment).toHaveBeenCalledTimes(2));
  });

  it("freezes attachment mutations and discard while create is pending", async () => {
    let resolveCreate: (value: { id: string }) => void = () => undefined;
    createTask.mockReturnValue(
      new Promise((resolve) => {
        resolveCreate = resolve;
      }),
    );
    uploadAttachment.mockResolvedValue(uploaded());
    renderDialog();
    fireEvent.change(screen.getByPlaceholderText(TITLE_PLACEHOLDER), {
      target: { value: "Frozen submission" },
    });
    selectFile();
    await waitFor(() => expect(screen.getByText("notes.txt")).toBeTruthy());
    const uploadsBefore = uploadAttachment.mock.calls.length;

    fireEvent.click(screen.getByTestId(CREATE_BUTTON));
    await waitFor(() => expect(createTask).toHaveBeenCalledTimes(1));

    selectFile("late.txt", "late");
    await act(async () => {
      await Promise.resolve();
    });
    expect(uploadAttachment).toHaveBeenCalledTimes(uploadsBefore);
    expect(screen.queryByText("late.txt")).toBeNull();
    expect(screen.queryAllByTestId("context-chip-remove")).toHaveLength(0);
    expect(screen.queryByTestId(RETRY_BUTTON)).toBeNull();
    const discard = screen.getByRole("button", { name: "Discard Draft" }) as HTMLButtonElement;
    expect(discard.disabled).toBe(true);
    fireEvent.click(screen.getByRole("button", { name: "Close" }));
    expect(screen.getByTestId("office-new-issue-dialog")).toBeTruthy();

    await act(async () => {
      resolveCreate({ id: "task-created" });
    });
    await waitFor(() => expect(createTask).toHaveBeenCalledTimes(1));
    expect(deleteAttachment).not.toHaveBeenCalled();
    expect(screen.queryByText("late.txt")).toBeNull();
  });

  it("disables Create for an expired restored attachment", () => {
    localStorage.setItem(
      `kandev:issue-draft:${WORKSPACE_ID}`,
      JSON.stringify({
        title: "Expired restore",
        description: "",
        assigneeId: "",
        projectId: "project-1",
        status: "todo",
        priority: "medium",
        attachments: [
          {
            id: "stored-expired",
            attachmentId: "attachment-expired",
            expiresAt: new Date(Date.now() - 60_000).toISOString(),
            mimeType: "text/plain",
            fileName: "expired.txt",
            size: 4,
            isImage: false,
            deliveryMode: "path",
          },
        ],
      }),
    );
    renderDialog();
    expect(screen.getByText("expired.txt")).toBeTruthy();
    expect((screen.getByTestId(CREATE_BUTTON) as HTMLButtonElement).disabled).toBe(true);
    fireEvent.click(screen.getByText("expired.txt"));
    expect(screen.queryByTestId(RETRY_BUTTON)).toBeNull();
  });
});
