import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { PreviewCaptureController } from "@/hooks/use-preview-capture";

const touch = vi.hoisted(() => ({ value: false }));
vi.mock("@/hooks/use-compact-task-chrome", () => ({ useTouchDrawer: () => touch.value }));

import { PreviewAnnotationLayer } from "./preview-annotation-layer";

const screenshotDraft = (uniformOverCanvas = false) => ({
  id: "draft-1",
  comment: "",
  open: true,
  payload: {
    kind: "screenshot",
    page_route: "/checkout",
    page_title: "Checkout",
    capture_rect: { x: 20, y: 40, width: 300, height: 180 },
    screenshot: {
      blob: new Blob([], { type: "image/png" }),
      width: 600,
      height: 360,
      previewUrl: "blob:preview-screenshot",
      fileName: "preview.png",
      uniformOverCanvas,
    },
  },
});

function controller(overrides: Record<string, unknown> = {}) {
  const annotation = {
    active: true,
    tool: "region",
    setTool: vi.fn(),
    exit: vi.fn(),
    toggle: vi.fn(),
    ...(overrides.annotation as object),
  };
  return {
    items: [],
    snapshot: { task_id: "task-1", revision: 0, items: [] },
    drafts: [],
    openDraft: null,
    draftAnchor: { x: 40, y: 120 },
    editingItem: null,
    editingAnchor: null,
    regions: [],
    selectionRect: null,
    limitReached: false,
    recaptureFailed: false,
    captureError: null,
    isRasterizing: false,
    isUploading: false,
    isMutating: false,
    mutationError: null,
    acknowledgedMode: "screenshot",
    pendingPanelOpen: false,
    setPendingPanelOpen: vi.fn(),
    openRegion: vi.fn(),
    recaptureRegion: vi.fn(),
    captureSelection: vi.fn(),
    setDraftComment: vi.fn(),
    closeDraftSurface: vi.fn(),
    saveDraft: vi.fn(),
    resumeDraft: vi.fn(),
    discardDraft: vi.fn(),
    closeItemEditor: vi.fn(),
    handleEscape: vi.fn(),
    update: vi.fn(),
    remove: vi.fn(),
    clear: vi.fn(),
    ...overrides,
    annotation,
  } as unknown as PreviewCaptureController;
}

function renderLayer(capture: PreviewCaptureController) {
  return render(<PreviewAnnotationLayer capture={capture} taskId="task-1" />);
}

beforeEach(() => {
  touch.value = false;
});
afterEach(cleanup);

// @covers AC-TASKS-WEB-PREVIEW-FEEDBACK-005.2
// @covers AC-TASKS-WEB-PREVIEW-FEEDBACK-005.3
describe("PreviewAnnotationLayer toolbar", () => {
  it("shows the tools with one pressed, the pending count, and Exit", () => {
    const capture = controller({ items: [{ id: "a" }], drafts: [screenshotDraft()] });
    renderLayer(capture);

    expect(screen.getByTestId("preview-tool-region").getAttribute("aria-pressed")).toBe("true");
    expect(screen.getByTestId("preview-tool-text").getAttribute("aria-pressed")).toBe("false");
    expect(screen.getByTestId("preview-annotation-count").textContent).toContain("2");

    fireEvent.click(screen.getByTestId("preview-tool-element"));
    expect(capture.annotation.setTool).toHaveBeenCalledWith("element");
    fireEvent.click(screen.getByTestId("preview-annotation-exit"));
    expect(capture.annotation.exit).toHaveBeenCalled();
  });

  it("renders nothing of the mode while inactive", () => {
    renderLayer(controller({ annotation: { active: false } }));
    expect(screen.queryByTestId("preview-annotation-toolbar")).toBeNull();
  });

  it("disables the Region tool and explains the screenshot limit", () => {
    renderLayer(controller({ limitReached: true }));
    expect(screen.getByTestId("preview-tool-region").hasAttribute("disabled")).toBe(true);
    expect(screen.getByTestId("preview-screenshot-limit")).toBeTruthy();
  });

  it("uses touch-sized controls on coarse pointers", () => {
    touch.value = true;
    renderLayer(controller());
    expect(screen.getByTestId("preview-tool-region").className).toContain("h-11");
  });
});

// @covers AC-TASKS-WEB-PREVIEW-FEEDBACK-005.6
describe("PreviewAnnotationLayer regions", () => {
  const regions = [
    { id: "saved", index: 0, rect: { x: 1, y: 2, width: 30, height: 20 }, pending: false },
    { id: "draft", index: 1, rect: { x: 40, y: 2, width: 30, height: 20 }, pending: true },
  ];

  it("draws regions with handles only in the Region tool", () => {
    const { rerender } = renderLayer(controller({ regions }));
    const outlines = screen.getAllByTestId("preview-region");
    expect(outlines[1].getAttribute("data-pending")).toBe("true");
    expect(screen.getAllByTestId("region-handle")).toHaveLength(8);

    rerender(
      <PreviewAnnotationLayer
        capture={controller({ regions, annotation: { tool: "interact" } })}
        taskId="task-1"
      />,
    );
    expect(screen.queryAllByTestId("region-handle")).toHaveLength(0);
  });

  it("shows a recapture failure", () => {
    renderLayer(controller({ recaptureFailed: true }));
    expect(screen.getByRole("alert").textContent).toMatch(/recapture/i);
  });
});

describe("PreviewAnnotationLayer draft surface", () => {
  it("shows the actual PNG before saving", () => {
    renderLayer(controller({ openDraft: screenshotDraft() }));

    const preview = screen.getByRole("img", { name: "Screenshot preview" });
    expect(preview.getAttribute("src")).toBe("blob:preview-screenshot");
    expect(screen.getByText("600 × 360 · PNG")).toBeTruthy();
  });

  // @covers AC-TASKS-WEB-PREVIEW-FEEDBACK-002.9
  it("warns, without blocking Save, when a canvas capture came out one colour", () => {
    const draft = { ...screenshotDraft(true), comment: "The counter is missing" };
    renderLayer(controller({ openDraft: draft }));

    expect(screen.getByText(/Canvas content may not have been captured/i)).toBeTruthy();
    expect(screen.getByRole("button", { name: "Add" }).hasAttribute("disabled")).toBe(false);
  });

  it("saves the typed comment", () => {
    const capture = controller({ openDraft: { ...screenshotDraft(), comment: "Clipped" } });
    renderLayer(capture);
    fireEvent.click(screen.getByRole("button", { name: "Add" }));
    expect(capture.saveDraft).toHaveBeenCalledWith("Clipped");
  });
});

// @covers AC-TASKS-WEB-PREVIEW-FEEDBACK-005.1
describe("PreviewAnnotationLayer comment on selection", () => {
  it("offers a comment button beside a page selection, even outside the mode", () => {
    const capture = controller({
      annotation: { active: false },
      selectionRect: { x: 10, y: 20, width: 40, height: 16 },
    });
    renderLayer(capture);

    fireEvent.click(screen.getByTestId("preview-selection-comment"));
    expect(capture.captureSelection).toHaveBeenCalled();
  });

  it("hides the button while a draft surface is open", () => {
    renderLayer(
      controller({
        selectionRect: { x: 10, y: 20, width: 40, height: 16 },
        openDraft: screenshotDraft(),
      }),
    );
    expect(screen.queryByTestId("preview-selection-comment")).toBeNull();
  });
});
