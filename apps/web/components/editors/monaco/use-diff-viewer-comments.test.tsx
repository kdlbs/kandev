import { act, cleanup, renderHook } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

const mocks = vi.hoisted(() => ({
  gutterOptions: [] as unknown[],
  viewZones: [] as unknown[],
  addComment: vi.fn(),
  removeComment: vi.fn(),
  updateComment: vi.fn(),
  setEditingComment: vi.fn(),
}));

vi.mock("@/components/diff/use-diff-comments", () => ({
  useDiffComments: () => ({
    comments: [],
    addComment: mocks.addComment,
    removeComment: mocks.removeComment,
    updateComment: mocks.updateComment,
    editingCommentId: null,
    setEditingComment: mocks.setEditingComment,
  }),
}));

vi.mock("@/hooks/use-gutter-comments", () => ({
  useGutterComments: (_editor: unknown, options: unknown) => {
    mocks.gutterOptions.push(options);
    return { clearGutterSelection: vi.fn() };
  },
}));

vi.mock("./use-diff-view-zones", () => ({
  useViewZones: (options: unknown) => {
    mocks.viewZones.push(options);
  },
}));

import { useDiffViewerComments } from "./use-diff-viewer-comments";

type GutterOptions = {
  enabled: boolean;
  onSelectionComplete: (params: {
    range: { start: number; end: number };
    code: string;
    position: { x: number; y: number };
  }) => void;
};

type ViewZoneOptions = {
  showCommentForm: boolean;
  selectedLineRange: { start: number; end: number; side: string } | null;
  submitDisabled?: boolean;
  handleCommentSubmitRef: { current: (content: string) => void };
  handleCommentSubmitAndRunRef?: { current: ((content: string) => void) | undefined };
};

function latestGutterOptions() {
  return mocks.gutterOptions.at(-2) as GutterOptions;
}

function latestViewZoneOptions() {
  return mocks.viewZones.at(-1) as ViewZoneOptions;
}

const data = { filePath: "src/example.ts", newContent: "new\n", oldContent: "old\n" };

function renderComments(ready: boolean, onCommentAdd = vi.fn(), onCommentRun = vi.fn()) {
  const options = {
    data,
    sessionId: "session-1",
    compact: false,
    enableComments: ready,
    externalComments: [],
    onCommentAdd,
    onCommentRun,
  } as Parameters<typeof useDiffViewerComments>[0];
  return {
    onCommentAdd,
    onCommentRun,
    ...renderHook(({ currentOptions }) => useDiffViewerComments(currentOptions), {
      initialProps: { currentOptions: options },
    }),
  };
}

beforeEach(() => {
  mocks.gutterOptions.length = 0;
  mocks.viewZones.length = 0;
  mocks.addComment.mockClear();
  mocks.removeComment.mockClear();
  mocks.updateComment.mockClear();
  mocks.setEditingComment.mockClear();
});

afterEach(cleanup);

describe("Monaco diff comment readiness", () => {
  it("blocks gutter selection while the displayed patch is stale", () => {
    renderComments(false);

    const gutter = latestGutterOptions();
    expect(gutter.enabled).toBe(false);
    act(() => {
      gutter.onSelectionComplete({
        range: { start: 7, end: 7 },
        code: "old patch line",
        position: { x: 0, y: 0 },
      });
    });
    expect(latestViewZoneOptions().showCommentForm).toBe(false);
    expect(latestViewZoneOptions().selectedLineRange).toBeNull();
  });

  it("preserves an open draft and rejects save until ready again", () => {
    const view = renderComments(true);
    act(() => {
      latestGutterOptions().onSelectionComplete({
        range: { start: 7, end: 7 },
        code: "current line",
        position: { x: 0, y: 0 },
      });
    });
    const readyZone = latestViewZoneOptions();
    expect(readyZone.showCommentForm).toBe(true);
    expect(readyZone.selectedLineRange).toEqual({ start: 7, end: 7, side: "additions" });

    view.rerender({
      currentOptions: {
        data,
        sessionId: "session-1",
        compact: false,
        enableComments: false,
        externalComments: [],
        onCommentAdd: view.onCommentAdd,
        onCommentRun: view.onCommentRun,
      } as Parameters<typeof useDiffViewerComments>[0],
    });
    const staleZone = latestViewZoneOptions();
    expect(staleZone.showCommentForm).toBe(true);
    expect(staleZone.selectedLineRange).toEqual(readyZone.selectedLineRange);
    expect(staleZone.submitDisabled).toBe(true);
    act(() => staleZone.handleCommentSubmitRef.current("draft"));
    expect(view.onCommentAdd).not.toHaveBeenCalled();
    expect(latestViewZoneOptions().showCommentForm).toBe(true);

    view.rerender({
      currentOptions: {
        data,
        sessionId: "session-1",
        compact: false,
        enableComments: true,
        externalComments: [],
        onCommentAdd: view.onCommentAdd,
        onCommentRun: view.onCommentRun,
      } as Parameters<typeof useDiffViewerComments>[0],
    });
    act(() => latestViewZoneOptions().handleCommentSubmitRef.current("draft"));
    expect(view.onCommentAdd).toHaveBeenCalledOnce();
    expect(latestViewZoneOptions().showCommentForm).toBe(false);
  });
});

describe("Monaco diff draft submission", () => {
  it("rejects submit-and-run from an open draft until ready again", () => {
    const view = renderComments(true);
    act(() => {
      latestGutterOptions().onSelectionComplete({
        range: { start: 8, end: 8 },
        code: "current line",
        position: { x: 0, y: 0 },
      });
    });

    view.rerender({
      currentOptions: {
        data,
        sessionId: "session-1",
        compact: false,
        enableComments: false,
        externalComments: [],
        onCommentAdd: view.onCommentAdd,
        onCommentRun: view.onCommentRun,
      } as Parameters<typeof useDiffViewerComments>[0],
    });
    const staleZone = latestViewZoneOptions();
    act(() => staleZone.handleCommentSubmitAndRunRef?.current?.("draft"));
    expect(view.onCommentAdd).not.toHaveBeenCalled();
    expect(view.onCommentRun).not.toHaveBeenCalled();
    expect(latestViewZoneOptions().showCommentForm).toBe(true);

    view.rerender({
      currentOptions: {
        data,
        sessionId: "session-1",
        compact: false,
        enableComments: true,
        externalComments: [],
        onCommentAdd: view.onCommentAdd,
        onCommentRun: view.onCommentRun,
      } as Parameters<typeof useDiffViewerComments>[0],
    });
    act(() => latestViewZoneOptions().handleCommentSubmitAndRunRef?.current?.("draft"));
    expect(view.onCommentAdd).toHaveBeenCalledOnce();
    expect(view.onCommentRun).toHaveBeenCalledOnce();
  });
});
