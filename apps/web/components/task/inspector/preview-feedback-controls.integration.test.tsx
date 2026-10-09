import { act, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { INSPECTOR_PROTOCOL_VERSION } from "@/lib/preview-inspect-bridge";
import type { TaskPreviewFeedback } from "@/lib/types/http";

const screenshotComment = "Keep this screenshot";

const feedback = vi.hoisted(() => ({
  items: [] as TaskPreviewFeedback[],
  snapshot: { task_id: "task-1", revision: 0, items: [] as TaskPreviewFeedback[] },
  create: vi.fn(),
  update: vi.fn(),
  remove: vi.fn(),
  clear: vi.fn(),
  isMutating: false,
  mutationError: null as string | null,
}));
vi.mock("@/hooks/domains/comments/use-preview-feedback", () => ({
  usePreviewFeedback: () => feedback,
}));
vi.mock("@/components/state-provider", () => ({
  useAppStore: (selector: (state: unknown) => unknown) =>
    selector({
      kanban: { tasks: [{ id: "task-1", workspaceId: "workspace-1" }] },
      kanbanMulti: { snapshots: {} },
      workspaces: { activeId: "workspace-1" },
    }),
}));

const attachments = vi.hoisted(() => ({
  uploadAttachment: vi.fn(),
  deleteAttachment: vi.fn(),
}));
vi.mock("@/lib/api/domains/attachment-api", () => attachments);

const html2canvas = vi.hoisted(() => vi.fn());
vi.mock("html2canvas-pro", () => ({ default: html2canvas }));

const bridge = vi.hoisted(() => ({
  sendSetPreviewCaptureMode: vi.fn(),
  sendProjectPreviewMarkers: vi.fn(),
}));
vi.mock("@/lib/preview-inspect-bridge", async () => {
  const actual = await vi.importActual<typeof import("@/lib/preview-inspect-bridge")>(
    "@/lib/preview-inspect-bridge",
  );
  return { ...actual, ...bridge };
});

vi.mock("@/hooks/use-compact-task-chrome", () => ({
  useTouchDrawer: () => false,
}));

import { PreviewFeedbackControls } from "./preview-feedback-controls";
import { usePreviewCapture } from "@/hooks/use-preview-capture";

function Harness({ iframeRef }: { iframeRef: React.RefObject<HTMLIFrameElement | null> }) {
  const capture = usePreviewCapture({
    taskId: "task-1",
    iframeRef,
    source: { kind: "browser", sessionId: "session-1", label: "Local app" },
    enabled: true,
  });
  return <PreviewFeedbackControls capture={capture} enabled />;
}

function dispatch(iframe: HTMLIFrameElement, data: unknown) {
  window.dispatchEvent(new MessageEvent("message", { source: iframe.contentWindow, data }));
}

describe("PreviewFeedbackControls with the capture controller", () => {
  beforeEach(() => {
    document.body.innerHTML = "";
    vi.clearAllMocks();
    feedback.items = [];
    feedback.snapshot = { task_id: "task-1", revision: 0, items: [] };
    feedback.create.mockResolvedValue(null);
    attachments.uploadAttachment.mockResolvedValue({ attachment_id: "attachment-1" });
    attachments.deleteAttachment.mockResolvedValue(undefined);
    html2canvas.mockResolvedValue({
      width: 320,
      height: 180,
      toBlob: (callback: BlobCallback) =>
        callback(
          new Blob([new Uint8Array([137, 80, 78, 71, 13, 10, 26, 10])], { type: "image/png" }),
        ),
    });
    Object.defineProperty(URL, "createObjectURL", {
      configurable: true,
      value: vi.fn(() => "blob:preview"),
    });
    Object.defineProperty(URL, "revokeObjectURL", {
      configurable: true,
      value: vi.fn(),
    });
  });

  afterEach(() => {
    document.body.innerHTML = "";
  });

  it("retains the comment and uploaded PNG after create fails, then retries without upload", async () => {
    const iframe = document.createElement("iframe");
    document.body.appendChild(iframe);
    const iframeRef = { current: iframe };
    render(<Harness iframeRef={iframeRef} />);

    let resolveCreate: ((value: null) => void) | undefined;
    feedback.create.mockImplementation(
      () => new Promise<null>((resolve) => (resolveCreate = resolve)),
    );

    act(() =>
      dispatch(iframe, {
        source: "kandev-inspector",
        version: INSPECTOR_PROTOCOL_VERSION,
        type: "screenshot-region-selected",
        payload: {
          page_route: "/checkout",
          page_title: "Checkout",
          capture_rect: { x: 0, y: 0, width: 320, height: 180 },
        },
      }),
    );
    await waitFor(() => expect(screen.getByTestId("preview-feedback-draft")).toBeTruthy());

    const comment = screen.getByLabelText("Comment on selection");
    fireEvent.change(comment, { target: { value: screenshotComment } });
    fireEvent.click(screen.getByRole("button", { name: "Save feedback" }));

    await waitFor(() =>
      expect(feedback.create).toHaveBeenCalledWith(
        expect.objectContaining({
          comment: screenshotComment,
          screenshotAttachmentId: "attachment-1",
        }),
      ),
    );
    await waitFor(() => expect(resolveCreate).toBeDefined());
    await act(async () => resolveCreate?.(null));
    expect((screen.getByLabelText("Comment on selection") as HTMLTextAreaElement).value).toBe(
      screenshotComment,
    );
    expect(screen.getByRole("img", { name: "Screenshot preview" }).getAttribute("src")).toBe(
      "blob:preview",
    );

    feedback.create.mockResolvedValue(null);
    fireEvent.click(screen.getByRole("button", { name: "Save feedback" }));
    await waitFor(() => expect(feedback.create).toHaveBeenCalledTimes(2));
    expect(attachments.uploadAttachment).toHaveBeenCalledTimes(1);
    expect(feedback.create).toHaveBeenLastCalledWith(
      expect.objectContaining({
        comment: screenshotComment,
        screenshotAttachmentId: "attachment-1",
      }),
    );
  });
});
