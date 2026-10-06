import { createRef, useCallback, useRef, useState, type ReactNode } from "react";
import { act, cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
import { TooltipProvider } from "@kandev/ui/tooltip";
import { ToastProvider } from "@/components/toast-provider";
import { processFile, type FileAttachment } from "@/components/task/chat/file-attachment";
import { useQuickChatSetupDraft } from "@/components/quick-chat/use-quick-chat-setup-draft";
import type { PluginComposerSlotProps } from "@/lib/plugins/types";
import type { TaskFormInputsHandle } from "./task-create-dialog-types";
import { TaskFormInputs } from "./task-create-dialog-selectors";

const FILE_NAME = "trace.txt";
const TEXT_MIME_TYPE = "text/plain";
const DESCRIPTION_INPUT_TEST_ID = "task-description-input";
const SWITCH_SETUP_TEST_ID = "switch-setup-tab";
const DRAFT_ATTACHMENT_STATUS_TEST_ID = "draft-attachment-status";
const API = vi.hoisted(() => ({ upload: vi.fn(), remove: vi.fn() }));
const mention = vi.hoisted(() => ({ handleKeyDown: vi.fn() }));
const pluginSlotCalls: PluginComposerSlotProps[] = [];

vi.mock("@/lib/api/domains/attachment-api", () => ({
  uploadAttachment: (...args: unknown[]) => API.upload(...args),
  deleteAttachment: (...args: unknown[]) => API.remove(...args),
}));

vi.mock("@/components/task/chat/file-attachment", async () => {
  const actual = await vi.importActual<typeof import("@/components/task/chat/file-attachment")>(
    "@/components/task/chat/file-attachment",
  );
  return { ...actual, processFile: vi.fn() };
});

vi.mock("@/components/plugins/plugin-slot", () => ({
  PluginSlot: ({ slotProps }: { slotProps: PluginComposerSlotProps }) => {
    pluginSlotCalls.push(slotProps);
    return null;
  },
}));
vi.mock("@/hooks/use-task-create-prompt-mention", () => {
  const useMention = ({ onChange }: { onChange?: (value: string) => void } = {}) => ({
    isOpen: false,
    isLoading: false,
    position: null,
    items: [],
    query: "",
    selectedIndex: 0,
    handleChange: (value: string) => onChange?.(value),
    handleKeyDown: mention.handleKeyDown,
    handleSelect: () => {},
    closeMenu: () => {},
    setSelectedIndex: () => {},
  });
  return {
    useTaskCreatePromptMention: useMention,
    useTaskCreatePromptMentionForInput: useMention,
  };
});

function Wrapper({ children }: { children: ReactNode }) {
  return (
    <ToastProvider>
      <TooltipProvider>{children}</TooltipProvider>
    </ToastProvider>
  );
}

function QuickChatSetupTabHarness() {
  const [showSetup, setShowSetup] = useState(true);
  const [sentAttachmentId, setSentAttachmentId] = useState("");
  const formRef = useRef<TaskFormInputsHandle>(null);
  const setupDraft = useQuickChatSetupDraft("workspace-upload-test", "user-upload-test");
  const attachments = setupDraft.draft.attachments;
  const onAttachmentsChange = useCallback(
    (next: FileAttachment[]) => setupDraft.update({ attachments: next }),
    [setupDraft.update],
  );

  return (
    <>
      <button data-testid="switch-setup-tab" onClick={() => setShowSetup((value) => !value)}>
        Switch tab
      </button>
      <button
        data-testid="discard-setup"
        onClick={() => {
          setupDraft.clear(true);
          setShowSetup(false);
        }}
      >
        Discard
      </button>
      <output data-testid="draft-attachment-status">
        {attachments[0]?.uploadStatus ?? "empty"}
      </output>
      <output data-testid="draft-attachment-id">{attachments[0]?.attachmentId ?? ""}</output>
      <output data-testid="sent-attachment-id">{sentAttachmentId}</output>
      {showSetup && (
        <>
          <TaskFormInputs
            workspaceId="workspace-upload-test"
            isSessionMode
            autoFocus={false}
            initialDescription="Review this file"
            initialAttachments={attachments}
            onDescriptionChange={() => {}}
            onAttachmentsChange={onAttachmentsChange}
            onKeyDown={() => {}}
            descriptionValueRef={formRef}
          />
          <button
            data-testid="submit-draft"
            disabled={attachments.some((attachment) => attachment.uploadStatus !== "ready")}
            onClick={() =>
              setSentAttachmentId(formRef.current?.getAttachments()[0]?.attachmentId ?? "")
            }
          >
            Send
          </button>
        </>
      )}
    </>
  );
}

function makeProcessedFile(file: File): FileAttachment {
  return {
    id: file.name,
    file,
    mimeType: file.type,
    fileName: file.name,
    size: file.size,
    isImage: false,
    deliveryMode: "path",
    uploadStatus: "pending",
  };
}

function uploadDescriptor(attachmentId: string) {
  return {
    attachment_id: attachmentId,
    name: FILE_NAME,
    mime_type: TEXT_MIME_TYPE,
    kind: "resource" as const,
    delivery_mode: "path" as const,
    size_bytes: 10,
  };
}

function lastPluginSlotProps(): PluginComposerSlotProps {
  const last = pluginSlotCalls.at(-1);
  if (!last) throw new Error("PluginSlot was not rendered");
  return last;
}

function chooseFile(container: HTMLElement, file: File) {
  const input = container.querySelector('input[type="file"]');
  if (!input) throw new Error("The composer file input is missing");
  fireEvent.change(input, { target: { files: [file] } });
}

afterEach(() => {
  cleanup();
  localStorage.clear();
  sessionStorage.clear();
  pluginSlotCalls.length = 0;
  mention.handleKeyDown.mockReset();
  API.upload.mockReset();
  API.remove.mockReset().mockResolvedValue(undefined);
  vi.mocked(processFile).mockReset();
});

it("uses the task-less Quick Chat plugin surface and submits its current draft", async () => {
  const submit = vi.fn(() => true);
  const formRef = createRef<TaskFormInputsHandle>();
  render(
    <TaskFormInputs
      isSessionMode
      quickChatComposer
      autoFocus={false}
      initialDescription=""
      onDescriptionChange={() => {}}
      onKeyDown={() => {}}
      descriptionValueRef={formRef}
      onComposerSubmit={submit}
    />,
    { wrapper: Wrapper },
  );

  const slot = lastPluginSlotProps();
  expect(slot.surface).toBe("quick-chat");
  expect(slot.taskId).toBeNull();
  expect(slot.activeSessionId).toBeNull();
  expect(slot.sessionIds).toEqual([]);
  await act(async () => {
    slot.composer.insertText("spoken opening");
    await slot.composer.submit();
  });
  expect(formRef.current?.getValue()).toBe("spoken opening");
  expect(submit).toHaveBeenCalledOnce();
});

it("restores staged attachments and reports opening draft edits", () => {
  const formRef = createRef<TaskFormInputsHandle>();
  const onAttachmentsChange = vi.fn();
  const onDescriptionValueChange = vi.fn();
  const attachment: FileAttachment = {
    id: "attachment-1",
    attachmentId: "staged-1",
    mimeType: "text/plain",
    fileName: "notes.txt",
    size: 12,
    isImage: false,
    deliveryMode: "path",
    uploadStatus: "ready",
  };
  render(
    <TaskFormInputs
      isSessionMode
      autoFocus={false}
      initialDescription="restored"
      initialAttachments={[attachment]}
      onDescriptionChange={() => {}}
      onDescriptionValueChange={onDescriptionValueChange}
      onAttachmentsChange={onAttachmentsChange}
      onKeyDown={() => {}}
      descriptionValueRef={formRef}
    />,
    { wrapper: Wrapper },
  );

  expect(formRef.current?.getAttachments()).toEqual([attachment]);
  fireEvent.change(screen.getByTestId(DESCRIPTION_INPUT_TEST_ID), {
    target: { value: "updated prompt" },
  });
  expect(onDescriptionValueChange).toHaveBeenCalledWith("updated prompt");
  expect(onAttachmentsChange).toHaveBeenLastCalledWith([attachment]);
});

it("keeps a completed upload ready when Quick Chat setup unmounts and returns", async () => {
  let resolveUpload!: (result: ReturnType<typeof uploadDescriptor>) => void;
  API.upload.mockImplementation(
    () =>
      new Promise((resolve) => {
        resolveUpload = resolve;
      }),
  );
  const file = new File(["trace data"], FILE_NAME, { type: TEXT_MIME_TYPE });
  vi.mocked(processFile).mockResolvedValue(makeProcessedFile(file));
  const view = render(<QuickChatSetupTabHarness />, { wrapper: Wrapper });

  chooseFile(view.container, file);
  await waitFor(() => expect(API.upload).toHaveBeenCalledOnce());
  fireEvent.click(screen.getByTestId(SWITCH_SETUP_TEST_ID));
  expect(screen.getByTestId(DRAFT_ATTACHMENT_STATUS_TEST_ID).textContent).toBe("uploading");

  await act(async () => resolveUpload(uploadDescriptor("staged-trace")));

  expect(screen.getByTestId(DRAFT_ATTACHMENT_STATUS_TEST_ID).textContent).toBe("ready");
  expect(screen.getByTestId("draft-attachment-id").textContent).toBe("staged-trace");
  fireEvent.click(screen.getByTestId(SWITCH_SETUP_TEST_ID));
  await waitFor(() =>
    expect((screen.getByTestId("submit-draft") as HTMLButtonElement).disabled).toBe(false),
  );
  fireEvent.click(screen.getByTestId("submit-draft"));
  expect(screen.getByTestId("sent-attachment-id").textContent).toBe("staged-trace");
});

it("restores a failed upload for retry and deletes a late success after discard", async () => {
  let rejectUpload!: (error: Error) => void;
  API.upload
    .mockImplementationOnce(
      () =>
        new Promise((_resolve, reject) => {
          rejectUpload = reject;
        }),
    )
    .mockRejectedValueOnce(new Error("retry failed"))
    .mockResolvedValueOnce(uploadDescriptor("retried-trace"));
  const file = new File(["trace data"], FILE_NAME, { type: TEXT_MIME_TYPE });
  vi.mocked(processFile).mockResolvedValue(makeProcessedFile(file));
  const view = render(<QuickChatSetupTabHarness />, { wrapper: Wrapper });

  chooseFile(view.container, file);
  await waitFor(() => expect(API.upload).toHaveBeenCalledTimes(1));
  fireEvent.click(screen.getByTestId(SWITCH_SETUP_TEST_ID));
  await act(async () => rejectUpload(new Error("upload refused")));
  fireEvent.click(screen.getByTestId(SWITCH_SETUP_TEST_ID));
  await waitFor(() => expect(API.upload).toHaveBeenCalledTimes(2));
  await waitFor(() =>
    expect(screen.getByTestId(DRAFT_ATTACHMENT_STATUS_TEST_ID).textContent).toBe("failed"),
  );
  fireEvent.click(screen.getByText(FILE_NAME));
  fireEvent.click(screen.getByTestId("attachment-upload-retry"));
  await waitFor(() =>
    expect(screen.getByTestId("draft-attachment-id").textContent).toBe("retried-trace"),
  );

  const discardFile = new File(["discard me"], "discard.txt", { type: TEXT_MIME_TYPE });
  vi.mocked(processFile).mockResolvedValue(makeProcessedFile(discardFile));
  let resolveDiscardedUpload!: (result: ReturnType<typeof uploadDescriptor>) => void;
  API.upload.mockImplementationOnce(
    () =>
      new Promise((resolve) => {
        resolveDiscardedUpload = resolve;
      }),
  );
  chooseFile(view.container, discardFile);
  await waitFor(() => expect(API.upload).toHaveBeenCalledTimes(4));
  fireEvent.click(screen.getByTestId("discard-setup"));
  await act(async () => resolveDiscardedUpload(uploadDescriptor("discarded-upload")));

  await waitFor(() => expect(API.remove).toHaveBeenCalledWith("discarded-upload"));
  expect(screen.getByTestId(DRAFT_ATTACHMENT_STATUS_TEST_ID).textContent).toBe("empty");
});
