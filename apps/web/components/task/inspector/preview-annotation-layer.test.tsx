import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { PreviewCaptureController } from "@/hooks/use-preview-capture";

const touch = vi.hoisted(() => ({ value: false }));
vi.mock("@/hooks/use-compact-task-chrome", () => ({ useTouchDrawer: () => touch.value }));

import { PreviewAnnotationHost } from "./preview-comment-panel";

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
    commentPanel: { open: false, setOpen: vi.fn(), toggle: vi.fn(), touch: false, docked: false },
    selectedId: null,
    selectCapture: vi.fn(),
    selectTool: vi.fn(),
    openCapture: vi.fn(),
    recaptureRegion: vi.fn(),
    captureSelection: vi.fn(),
    setDraftComment: vi.fn(),
    closeComment: vi.fn(),
    captureNumbers: new Map(),
    saveDraft: vi.fn(),

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
  return render(
    <PreviewAnnotationHost capture={capture} showLayer taskId="task-1">
      <iframe title="page" />
    </PreviewAnnotationHost>,
  );
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
    expect(capture.selectTool).toHaveBeenCalledWith("element");
    fireEvent.click(screen.getByTestId("preview-annotation-exit"));
    expect(capture.annotation.exit).toHaveBeenCalled();
  });

  it("offers a drag handle on fine pointers only", () => {
    renderLayer(controller());
    const handle = screen.getByTestId("preview-annotation-toolbar-handle");
    expect(handle.getAttribute("aria-label")).toBe("Move toolbar");
    cleanup();

    touch.value = true;
    renderLayer(controller());
    expect(screen.queryByTestId("preview-annotation-toolbar-handle")).toBeNull();
  });

  it("stacks the toolbar above the overlay panel, outside the annotation layer", () => {
    renderLayer(controller());
    const toolbar = screen.getByTestId("preview-annotation-toolbar");
    expect(screen.getByTestId("preview-annotation-layer").contains(toolbar)).toBe(false);
    expect(screen.getByTestId("preview-annotation-frame").contains(toolbar)).toBe(true);
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
    {
      id: "saved",
      index: 0,
      number: 1,
      rect: { x: 1, y: 2, width: 30, height: 20 },
      pending: false,
      busy: false,
    },
    {
      id: "draft",
      index: 1,
      number: 3,
      rect: { x: 40, y: 2, width: 30, height: 20 },
      pending: true,
      busy: true,
    },
  ];

  it("numbers regions like the list and opens one from its badge in any tool", () => {
    const capture = controller({ regions, annotation: { tool: "interact" } });
    renderLayer(capture);
    const outlines = screen.getAllByTestId("preview-region");
    expect(outlines[1].getAttribute("data-busy")).toBe("true");
    const badge = outlines[1].querySelector("[data-region-badge]") as HTMLElement;
    expect(badge.textContent).toBe("3");

    fireEvent.click(badge);
    expect(capture.openCapture).toHaveBeenCalledWith("draft");
  });

  it("disables Add while the region is still being captured", () => {
    const draft = screenshotDraft();
    const capturing = { ...draft, payload: { ...draft.payload, screenshot: null } };
    renderLayer(controller({ openDraft: capturing }));
    expect(screen.getByTestId("preview-feedback-evidence-capturing")).toBeTruthy();
    expect(screen.getByRole("button", { name: "Add" }).hasAttribute("disabled")).toBe(true);
  });

  it("offers Close on a new capture's comment", () => {
    const capture = controller({ openDraft: screenshotDraft() });
    renderLayer(capture);
    fireEvent.click(screen.getByRole("button", { name: "Close" }));
    expect(capture.closeComment).toHaveBeenCalled();
  });

  it("draws regions with handles only in the Region tool", () => {
    const { rerender } = renderLayer(controller({ regions }));
    const outlines = screen.getAllByTestId("preview-region");
    expect(outlines[1].getAttribute("data-pending")).toBe("true");
    expect(screen.getAllByTestId("region-handle")).toHaveLength(8);

    rerender(
      <PreviewAnnotationHost
        capture={controller({ regions, annotation: { tool: "interact" } })}
        showLayer
        taskId="task-1"
      >
        <iframe title="page" />
      </PreviewAnnotationHost>,
    );
    expect(screen.queryAllByTestId("region-handle")).toHaveLength(0);
  });

  // @covers AC-TASKS-WEB-PREVIEW-FEEDBACK-007.3
  // @covers AC-TASKS-WEB-PREVIEW-FEEDBACK-007.4
  it("warns about a drifted region and shows a saved capture on request", () => {
    renderLayer(
      controller({
        regions: [
          { ...regions[0], drifted: true, imageUrl: "blob:saved" },
          { ...regions[1], drifted: false },
        ],
      }),
    );
    const [drifted, aligned] = screen.getAllByTestId("preview-region");
    expect(drifted.getAttribute("data-drifted")).toBe("true");
    expect(aligned.getAttribute("data-drifted")).toBeNull();
    expect(
      screen.getByRole("button", { name: /page layout changed since this capture/ }),
    ).toBeTruthy();

    fireEvent.click(screen.getByRole("button", { name: "Show capture" }));
    expect(screen.getByTestId("region-capture-image").getAttribute("src")).toBe("blob:saved");
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

const COMMENT_PANEL = "preview-comment-panel";

const savedItem = {
  id: "saved-1",
  kind: "text",
  comment: "Make it bigger",
  page_route: "/checkout",
  source_label: "localhost",
  selected_text: "Buy now",
  version: 1,
};

function panelController(overrides: Record<string, unknown> = {}) {
  return controller({
    items: [savedItem],
    captureNumbers: new Map([
      ["saved-1", 1],
      ["draft-1", 2],
    ]),
    commentPanel: { open: true, setOpen: vi.fn(), toggle: vi.fn(), touch: false, docked: true },
    ...overrides,
  });
}

const renderHost = renderLayer;

// @covers AC-TASKS-WEB-PREVIEW-FEEDBACK-006.1
describe("PreviewAnnotationLayer comment panel toggle", () => {
  it("toggles the panel from a pressed count button", () => {
    const capture = panelController();
    renderLayer(capture);
    const toggle = screen.getByTestId("preview-annotation-count");
    expect(toggle.getAttribute("aria-pressed")).toBe("true");
    expect(toggle.textContent).toContain("Hide comments");

    fireEvent.click(toggle);
    expect(capture.commentPanel.toggle).toHaveBeenCalled();
  });

  it("docks the panel beside the page, or overlays it on a narrow host", () => {
    const width = vi.spyOn(HTMLElement.prototype, "clientWidth", "get").mockReturnValue(1000);
    const { unmount } = renderHost(panelController());
    expect(screen.getByTestId(COMMENT_PANEL).getAttribute("data-overlay")).toBe("false");
    unmount();

    width.mockReturnValue(500);
    renderHost(panelController());
    expect(screen.getByTestId(COMMENT_PANEL).getAttribute("data-overlay")).toBe("true");
    width.mockRestore();
  });

  it("gives the page the full width when the panel is hidden", () => {
    renderHost(
      panelController({
        commentPanel: {
          open: false,
          setOpen: vi.fn(),
          toggle: vi.fn(),
          touch: false,
          docked: false,
        },
      }),
    );
    expect(screen.queryByTestId(COMMENT_PANEL)).toBeNull();
  });
});

// @covers AC-TASKS-WEB-PREVIEW-FEEDBACK-006.2
// @covers AC-TASKS-WEB-PREVIEW-FEEDBACK-006.3
describe("PreviewAnnotationLayer comment list", () => {
  it("writes a new capture's comment in the floating box while the panel is open", () => {
    const draft = screenshotDraft();
    renderHost(panelController({ drafts: [draft], openDraft: draft }));
    const panel = screen.getByTestId(COMMENT_PANEL);
    expect(panel.querySelector('[data-testid="preview-feedback-draft-input"]')).toBeNull();
    const surfaces = screen.getAllByTestId("preview-feedback-draft");
    expect(surfaces).toHaveLength(1);
    expect(panel.contains(surfaces[0])).toBe(false);
    expect(surfaces[0].className).not.toContain("animate-in");
    expect(
      panel.querySelector('[data-testid="preview-unsaved-draft"]')?.getAttribute("data-active"),
    ).toBe("true");
  });

  it("numbers entries like their markers and edits a saved item in the floating box", () => {
    const capture = panelController();
    const { rerender } = renderHost(capture);
    const item = screen.getByTestId("preview-feedback-item");
    expect(item.querySelector('[data-testid="preview-capture-number"]')?.textContent).toBe("1");

    fireEvent.click(item);
    expect(capture.selectCapture).toHaveBeenCalledWith("saved-1");
    fireEvent.click(screen.getByRole("button", { name: "Edit feedback" }));
    expect(capture.openCapture).toHaveBeenCalledWith("saved-1");

    const editing = panelController({ editingItem: savedItem });
    rerender(
      <PreviewAnnotationHost capture={editing} showLayer taskId="task-1">
        <iframe title="page" />
      </PreviewAnnotationHost>,
    );
    const panel = screen.getByTestId(COMMENT_PANEL);
    expect(panel.querySelector('[data-testid="preview-feedback-edit-input"]')).toBeNull();
    const editors = screen.getAllByTestId("preview-feedback-edit");
    expect(editors).toHaveLength(1);
    expect(panel.contains(editors[0])).toBe(false);
    expect(editors[0].className).not.toContain("animate-in");
    expect(screen.getByTestId("preview-feedback-item").getAttribute("data-active")).toBe("true");
    expect(screen.getByTestId("preview-feedback-item").textContent).toContain("Make it bigger");
  });

  it("hides the panel from its header", () => {
    const capture = panelController();
    renderHost(capture);
    fireEvent.click(screen.getByTestId("preview-comment-panel-hide"));
    expect(capture.commentPanel.setOpen).toHaveBeenCalledWith(false);
  });
});
