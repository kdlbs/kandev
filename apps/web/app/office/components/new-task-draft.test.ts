import { act, renderHook } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { useIssueDraft } from "./new-task-draft";

const WORKSPACE_ID = "workspace-office-draft";
const KEY = `kandev:issue-draft:${WORKSPACE_ID}`;

const STORED_ATTACHMENT = {
  id: "stored-1",
  attachmentId: "attachment-1",
  expiresAt: "2099-01-01T00:00:00.000Z",
  mimeType: "image/png",
  fileName: "screenshot.png",
  size: 4,
  isImage: true,
  deliveryMode: "prompt" as const,
};

beforeEach(() => {
  localStorage.clear();
  vi.useFakeTimers();
});

afterEach(() => {
  vi.useRealTimers();
});

describe("useIssueDraft attachments", () => {
  it("restores ready descriptors from the saved workspace draft", () => {
    localStorage.setItem(
      KEY,
      JSON.stringify({
        title: "Saved",
        description: "",
        assigneeId: "",
        projectId: "project-1",
        status: "todo",
        priority: "medium",
        attachments: [STORED_ATTACHMENT],
      }),
    );
    const { result } = renderHook(() => useIssueDraft(WORKSPACE_ID));
    expect(result.current.draft.title).toBe("Saved");
    expect(result.current.draft.attachments).toEqual([STORED_ATTACHMENT]);
  });

  it("treats a legacy draft without attachments as empty", () => {
    localStorage.setItem(
      KEY,
      JSON.stringify({
        title: "Legacy",
        description: "",
        assigneeId: "",
        projectId: "project-1",
        status: "todo",
        priority: "medium",
      }),
    );
    const { result } = renderHook(() => useIssueDraft(WORKSPACE_ID));
    expect(result.current.draft.attachments).toEqual([]);
  });

  it("flushes the outgoing draft before a keyed owner switch unmounts it", () => {
    const { result, unmount } = renderHook(() => useIssueDraft(WORKSPACE_ID));
    act(() =>
      result.current.updateDraft({ title: "Latest edit", attachments: [STORED_ATTACHMENT] }),
    );
    unmount();
    const saved = JSON.parse(localStorage.getItem(KEY) ?? "{}");
    expect(saved.title).toBe("Latest edit");
    expect(saved.attachments).toEqual([STORED_ATTACHMENT]);
  });

  it("does not repopulate a discarded draft after unmount", () => {
    const { result, unmount } = renderHook(() => useIssueDraft(WORKSPACE_ID));
    act(() => result.current.updateDraft({ title: "Discarded" }));
    act(() => result.current.clearDraft());
    unmount();
    expect(localStorage.getItem(KEY)).toBeNull();
  });

  it("persists ready descriptors and clears them on discard", () => {
    const { result } = renderHook(() => useIssueDraft(WORKSPACE_ID));
    act(() => result.current.updateDraft({ title: "Draft", attachments: [STORED_ATTACHMENT] }));
    act(() => vi.advanceTimersByTime(800));
    const saved = JSON.parse(localStorage.getItem(KEY) ?? "{}");
    expect(saved.attachments).toEqual([STORED_ATTACHMENT]);

    act(() => result.current.clearDraft());
    expect(localStorage.getItem(KEY)).toBeNull();
    expect(result.current.draft.attachments).toEqual([]);
  });
});
