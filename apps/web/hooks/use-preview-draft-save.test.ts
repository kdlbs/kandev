import { act, renderHook } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";

const attachments = vi.hoisted(() => ({ uploadAttachment: vi.fn(), deleteAttachment: vi.fn() }));
vi.mock("@/lib/api/domains/attachment-api", () => attachments);
vi.mock("html2canvas-pro", () => ({ default: vi.fn() }));

import { usePreviewDraftSave } from "./use-preview-draft-save";
import { usePreviewDrafts, type PreviewScreenshotDraft } from "./use-preview-drafts";

function shot(width: number, x: number): PreviewScreenshotDraft {
  return {
    kind: "screenshot",
    page_route: "/",
    page_title: "Home",
    capture_rect: { x, y: 20, width, height: 50 },
    screenshot: {
      blob: new Blob([String(width)]),
      width,
      height: 50,
      previewUrl: `blob:${width}`,
      fileName: `shot-${width}.png`,
    },
  };
}

function deferred<T>() {
  let resolve!: (value: T) => void;
  const promise = new Promise<T>((res) => {
    resolve = res;
  });
  return { promise, resolve };
}

function renderSave(createFeedback: Parameters<typeof usePreviewDraftSave>[0]["createFeedback"]) {
  return renderHook(() => {
    const drafts = usePreviewDrafts();
    const save = usePreviewDraftSave({
      drafts,
      workspaceId: "ws",
      source: { kind: "browser", label: "http://localhost:3000" },
      createFeedback,
      setCaptureError: vi.fn(),
    });
    return { drafts, save };
  });
}

beforeEach(() => {
  vi.clearAllMocks();
  URL.revokeObjectURL = vi.fn();
  attachments.deleteAttachment.mockResolvedValue(undefined);
});

describe("usePreviewDraftSave", () => {
  it("uploads and saves the image the draft has when its first upload finishes stale", async () => {
    const firstUpload = deferred<{ attachment_id: string }>();
    attachments.uploadAttachment
      .mockReturnValueOnce(firstUpload.promise)
      .mockResolvedValueOnce({ attachment_id: "current-image" });
    const createFeedback = vi.fn().mockResolvedValue({ items: [] });
    const { result } = renderSave(createFeedback);
    let id = "";
    act(() => {
      id = result.current.drafts.add(shot(100, 10));
    });

    let saving: Promise<boolean> = Promise.resolve(false);
    act(() => {
      saving = result.current.save.saveDraft("Make it larger");
    });
    act(() => result.current.drafts.replaceScreenshot(id, shot(80, 40)));
    await act(async () => {
      firstUpload.resolve({ attachment_id: "stale-image" });
      await saving;
    });

    expect(attachments.deleteAttachment).toHaveBeenCalledWith("stale-image");
    expect(createFeedback).toHaveBeenCalledWith(
      expect.objectContaining({
        screenshotAttachmentId: "current-image",
        captureRect: expect.objectContaining({ x: 40, width: 80 }),
      }),
    );
  });
});
