import { afterEach, describe, expect, it, vi } from "vitest";
import { act } from "@testing-library/react";
import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { TooltipProvider } from "@kandev/ui/tooltip";
import { MarkdownPreviewContent } from "./markdown-preview-content";

const responsive = vi.hoisted(() => ({ isMobile: true, isFinePointer: true }));
const toast = vi.hoisted(() => vi.fn());

vi.mock("@/hooks/use-responsive-breakpoint", () => ({
  useResponsiveBreakpoint: () => ({ ...responsive }),
}));
vi.mock("@/hooks/domains/comments/use-run-comment", () => ({
  useRunComment: () => ({ runComment: vi.fn() }),
}));
vi.mock("@/hooks/domains/comments/use-diff-comments", () => ({
  useDiffFileComments: () => [],
}));
vi.mock("@/components/toast-provider", () => ({ useToast: () => ({ toast }) }));
vi.mock("@/components/editors/external-vcs-file-link", () => ({
  ExternalVcsFileLink: () => null,
  useExternalVcsFileStatus: () => null,
}));

const CONTENT = "A selected paragraph for comment.";
const FILE_PATH = "README.md";
const OWNER_SELECTOR = ".flex-1.overflow-auto";
const BODY_SELECTOR = ".markdown-body";
const PREVIEW_TEST_ID = "markdown-preview";
const DOCK_TEST_ID = "selection-comment-dock";
const ACTION_TEST_ID = "markdown-preview-comment-button";

function selectMarkdownText(root: HTMLElement, start: number, end: number) {
  const text = root.querySelector("p")!.firstChild!;
  const range = document.createRange();
  range.setStart(text, start);
  range.setEnd(text, end);
  const selection = window.getSelection()!;
  selection.removeAllRanges();
  selection.addRange(range);
}

function dispatchSelectionChange() {
  fireEvent(document, new Event("selectionchange"));
}

function renderPreview(path = FILE_PATH) {
  return render(
    <TooltipProvider>
      <MarkdownPreviewContent
        path={path}
        content={CONTENT}
        sessionId="session-1"
        taskId="task-1"
        repositoryId="repo-1"
        enableComments
        showExternalVcsLink={false}
        onTogglePreview={vi.fn()}
      />
    </TooltipProvider>,
  );
}

function setPreviewOwnerBounds() {
  const owner = screen.getByTestId(PREVIEW_TEST_ID).querySelector<HTMLElement>(OWNER_SELECTOR)!;
  vi.spyOn(owner, "getBoundingClientRect").mockReturnValue({
    left: 20,
    top: 100,
    right: 420,
    bottom: 700,
    width: 400,
    height: 600,
    x: 20,
    y: 100,
    toJSON: () => ({}),
  });
}

afterEach(() => {
  cleanup();
  vi.useRealTimers();
  responsive.isMobile = true;
  responsive.isFinePointer = true;
  toast.mockClear();
  vi.restoreAllMocks();
});

describe("MarkdownPreviewContent selection comments", () => {
  it("keeps the touch Comment action at the preview owner while the quote changes", async () => {
    renderPreview();
    const preview = screen.getByTestId(PREVIEW_TEST_ID);
    const root = preview.querySelector<HTMLElement>(BODY_SELECTOR);
    expect(root).not.toBeNull();
    setPreviewOwnerBounds();

    fireEvent.touchStart(root!);
    selectMarkdownText(root!, 2, 10);
    dispatchSelectionChange();
    fireEvent.touchEnd(root!);

    const action = await screen.findByTestId(ACTION_TEST_ID);
    const firstPosition = screen.getByTestId(DOCK_TEST_ID).getAttribute("style");
    expect(firstPosition).toContain("top: 644px");
    expect(action.getAttribute("aria-label")).toBe("Comment on selection");

    selectMarkdownText(root!, 11, CONTENT.length);
    dispatchSelectionChange();
    await act(async () => {
      await new Promise((resolve) => window.setTimeout(resolve, 20));
    });
    expect(screen.getByTestId(DOCK_TEST_ID).getAttribute("style")).toBe(firstPosition);
    fireEvent.pointerDown(action, { pointerType: "touch" });
    fireEvent.click(action);

    expect(document.querySelector("[data-markdown-comment-popover]")?.textContent).toContain(
      CONTENT.slice(11),
    );
  });
});

describe("Markdown selection dismissal", () => {
  it("cancels a pending touch capture when an outside pointer dismisses the dock", async () => {
    const outside = document.createElement("button");
    document.body.append(outside);
    renderPreview();
    setPreviewOwnerBounds();
    const root = screen.getByTestId(PREVIEW_TEST_ID).querySelector<HTMLElement>(BODY_SELECTOR)!;
    fireEvent.touchStart(root);
    selectMarkdownText(root, 2, 10);
    dispatchSelectionChange();
    fireEvent.touchEnd(root);
    await screen.findByTestId(DOCK_TEST_ID);

    vi.useFakeTimers();
    act(() => {
      dispatchSelectionChange();
      fireEvent.pointerDown(outside, { pointerType: "mouse" });
      vi.advanceTimersByTime(20);
    });

    expect(screen.queryByTestId(DOCK_TEST_ID)).toBeNull();
    expect(screen.queryByTestId(ACTION_TEST_ID)).toBeNull();
    outside.remove();
  });
});

describe("Markdown selection editor handoff", () => {
  it("keeps the validated quote when a pending capture hands off to the editor", async () => {
    renderPreview();
    setPreviewOwnerBounds();
    const root = screen.getByTestId(PREVIEW_TEST_ID).querySelector<HTMLElement>(BODY_SELECTOR)!;
    fireEvent.touchStart(root);
    selectMarkdownText(root, 2, 10);
    dispatchSelectionChange();
    fireEvent.touchEnd(root);
    const action = await screen.findByTestId(ACTION_TEST_ID);

    vi.useFakeTimers();
    act(() => {
      dispatchSelectionChange();
      fireEvent.pointerDown(action, { pointerType: "touch" });
      fireEvent.click(action);
      vi.advanceTimersByTime(20);
    });
    const popover = document.querySelector("[data-markdown-comment-popover]");
    expect(popover?.querySelector("pre")?.textContent).toBe(CONTENT.slice(2, 10));
    expect(screen.queryByTestId(DOCK_TEST_ID)).toBeNull();

    act(() => {
      fireEvent.keyDown(document.body, { key: "Escape" });
      vi.advanceTimersByTime(20);
    });
    expect(document.querySelector("[data-markdown-comment-popover]")).toBeNull();
    expect(screen.queryByTestId(DOCK_TEST_ID)).toBeNull();
  });
});

describe("Markdown selection identity changes", () => {
  it("cancels a pending capture when the preview file changes", async () => {
    const view = renderPreview();
    setPreviewOwnerBounds();
    const root = screen.getByTestId(PREVIEW_TEST_ID).querySelector<HTMLElement>(BODY_SELECTOR)!;
    fireEvent.touchStart(root);
    selectMarkdownText(root, 2, 10);
    dispatchSelectionChange();
    fireEvent.touchEnd(root);
    await screen.findByTestId(DOCK_TEST_ID);

    vi.useFakeTimers();
    act(() => {
      dispatchSelectionChange();
      view.rerender(
        <TooltipProvider>
          <MarkdownPreviewContent
            path="CONTRIBUTING.md"
            content={CONTENT}
            sessionId="session-1"
            taskId="task-1"
            repositoryId="repo-1"
            enableComments
            showExternalVcsLink={false}
            onTogglePreview={vi.fn()}
          />
        </TooltipProvider>,
      );
      vi.advanceTimersByTime(20);
    });

    expect(screen.queryByTestId(DOCK_TEST_ID)).toBeNull();
  });
});
