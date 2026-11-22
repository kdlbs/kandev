import { act, renderHook } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";

const attachments = vi.hoisted(() => ({ deleteAttachment: vi.fn() }));
vi.mock("@/lib/api/domains/attachment-api", () => attachments);

import type { PreviewFeedbackDraft, PreviewScreenshotDraft } from "./use-preview-drafts";
import { usePreviewDrafts } from "./use-preview-drafts";

const text: PreviewFeedbackDraft = {
  kind: "text",
  page_route: "/",
  page_title: "Home",
  selected_text: "Buy",
  text_anchor: {
    start: { node_path: [0], offset: 0 },
    end: { node_path: [0], offset: 3 },
    containing_element: { tag: "p", classes: [], outer_html: "<p>Buy</p>" },
  },
};

function screenshot(url: string, attachmentId?: string): PreviewScreenshotDraft {
  return {
    kind: "screenshot",
    page_route: "/",
    page_title: "Home",
    capture_rect: { x: 1, y: 2, width: 30, height: 40 },
    screenshot: {
      blob: new Blob(),
      width: 30,
      height: 40,
      previewUrl: url,
      fileName: "preview.png",
      attachmentId,
    },
  };
}

beforeEach(() => {
  vi.clearAllMocks();
  attachments.deleteAttachment.mockResolvedValue(undefined);
  Object.defineProperty(URL, "revokeObjectURL", { configurable: true, value: vi.fn() });
});

describe("usePreviewDrafts", () => {
  it("appends each capture and opens only the newest", () => {
    const { result } = renderHook(() => usePreviewDrafts());

    act(() => void result.current.add(text));
    act(() => void result.current.add(screenshot("blob:a")));

    expect(result.current.drafts).toHaveLength(2);
    expect(result.current.drafts.map((draft) => draft.open)).toEqual([false, true]);
    expect(result.current.openDraft?.payload.kind).toBe("screenshot");
  });

  it("keeps a dismissed draft and its comment, and resumes it", () => {
    const { result } = renderHook(() => usePreviewDrafts());
    let id = "";
    act(() => {
      id = result.current.add(text);
    });
    act(() => result.current.setComment(id, "Wording"));
    act(() => result.current.close());

    expect(result.current.openDraft).toBeNull();
    expect(result.current.drafts[0].comment).toBe("Wording");

    act(() => result.current.open(id));
    expect(result.current.openDraft?.id).toBe(id);
  });

  it("releases the PNG and staged upload when a draft is discarded", () => {
    const { result } = renderHook(() => usePreviewDrafts());
    let id = "";
    act(() => {
      id = result.current.add(screenshot("blob:a", "staged-1"));
    });

    act(() => result.current.discard(id));

    expect(result.current.drafts).toHaveLength(0);
    expect(URL.revokeObjectURL).toHaveBeenCalledWith("blob:a");
    expect(attachments.deleteAttachment).toHaveBeenCalledWith("staged-1");
  });

  it("removes a saved draft without deleting its claimed upload", () => {
    const { result } = renderHook(() => usePreviewDrafts());
    let id = "";
    act(() => {
      id = result.current.add(screenshot("blob:a", "claimed-1"));
    });

    act(() => result.current.complete(id));

    expect(result.current.drafts).toHaveLength(0);
    expect(URL.revokeObjectURL).toHaveBeenCalledWith("blob:a");
    expect(attachments.deleteAttachment).not.toHaveBeenCalled();
  });

  it("replaces a draft's screenshot and drops the previous image", () => {
    const { result } = renderHook(() => usePreviewDrafts());
    let id = "";
    act(() => {
      id = result.current.add(screenshot("blob:a", "staged-1"));
    });
    act(() => result.current.setComment(id, "Keep me"));

    const next = screenshot("blob:b");
    act(() => result.current.replaceScreenshot(id, next));

    expect(result.current.drafts[0].payload).toBe(next);
    expect(result.current.drafts[0].comment).toBe("Keep me");
    expect(URL.revokeObjectURL).toHaveBeenCalledWith("blob:a");
    expect(attachments.deleteAttachment).toHaveBeenCalledWith("staged-1");
  });

  it("releases every draft when the preview unmounts", () => {
    const { result, unmount } = renderHook(() => usePreviewDrafts());
    act(() => void result.current.add(screenshot("blob:a", "staged-1")));

    unmount();

    expect(URL.revokeObjectURL).toHaveBeenCalledWith("blob:a");
    expect(attachments.deleteAttachment).toHaveBeenCalledWith("staged-1");
  });
});
