import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { act, cleanup, fireEvent, render, screen } from "@testing-library/react";
import { resetMermaidErrorToastHistoryForTest } from "./mermaid-error-toast";
import { MemoizedMarkdown } from "./memoized-markdown";

const mockToast = vi.hoisted(() => vi.fn());
const renderInputs = vi.hoisted(() => [] as string[]);
const mermaidOperationOrder = vi.hoisted(() => [] as string[]);
const MERMAID_FENCE = "```mermaid";
const FLOWCHART_LR = "flowchart LR";
const renderControl = vi.hoisted(() => ({
  resolveOld: null as ((result: { svg: string }) => void) | null,
  oldRenderStarted: vi.fn(),
}));

vi.mock("@/components/theme/app-theme", () => ({
  useTheme: () => ({ resolvedTheme: "dark" }),
}));

vi.mock("@/components/toast-provider", () => ({
  useToast: () => ({ toast: mockToast, updateToast: vi.fn(), dismissToast: vi.fn() }),
}));

vi.mock("mermaid", () => ({
  default: {
    initialize: vi.fn(() => mermaidOperationOrder.push("initialize")),
    parse: vi.fn(async (code: string) => {
      mermaidOperationOrder.push("parse");
      return !code.includes("```") && !/A -->\s*$/.test(code);
    }),
    render: vi.fn(async (_id: string, code: string) => {
      mermaidOperationOrder.push("render");
      renderInputs.push(code);
      if (code.includes("OldNode")) {
        renderControl.oldRenderStarted();
        return await new Promise<{ svg: string }>((resolve) => {
          renderControl.resolveOld = resolve;
        });
      }
      if (code.includes("```") || /A -->\s*$/.test(code)) {
        throw new Error("Parse error");
      }
      const diagram = code.includes("NewNode") ? "new" : "diagram";
      return { svg: `<svg data-diagram="${diagram}"></svg>` };
    }),
  },
}));

const GLUED_MESSAGE = [
  MERMAID_FENCE,
  FLOWCHART_LR,
  "  A --> B",
  "```Following [guide](src/guide.md).",
].join("\n");

beforeEach(() => {
  vi.useFakeTimers();
  renderInputs.length = 0;
  mermaidOperationOrder.length = 0;
  renderControl.resolveOld = null;
  renderControl.oldRenderStarted.mockClear();
  mockToast.mockClear();
  resetMermaidErrorToastHistoryForTest();
});

afterEach(() => {
  cleanup();
  vi.useRealTimers();
});

async function flushRender(): Promise<void> {
  await act(async () => {
    vi.advanceTimersByTime(500);
    await Promise.resolve();
    await Promise.resolve();
    await Promise.resolve();
  });
}

describe("chat Markdown Mermaid boundary recovery", () => {
  // @covers AC-UI-MERMAID-RENDERING-002.1, AC-UI-MERMAID-RENDERING-002.6, AC-UI-MERMAID-RENDERING-002.7
  it("recovers prose glued to a closing Mermaid fence with the current file-link context", async () => {
    const openFile = vi.fn();
    const { container } = render(
      <MemoizedMarkdown
        content={GLUED_MESSAGE}
        taskId="mermaid-recovery-task"
        worktreePath="/workspace"
        onOpenFile={openFile}
      />,
    );

    await flushRender();

    const diagram = container.querySelector(".mermaid-block");
    expect(container.textContent).not.toContain("Failed to render diagram");
    expect(diagram).not.toBeNull();
    expect(diagram?.querySelector("svg")).not.toBeNull();
    expect(diagram?.closest("pre")).toBeNull();
    expect(diagram?.nextElementSibling?.tagName).toBe("P");
    expect(renderInputs).toEqual([`${FLOWCHART_LR}\n  A --> B\n`]);
    expect(mermaidOperationOrder).toEqual(["parse", "parse", "initialize", "render"]);
    expect(mockToast).not.toHaveBeenCalled();

    fireEvent.click(screen.getByTitle("Show code"));
    expect(diagram?.querySelector("pre code")?.textContent).toBe(`${FLOWCHART_LR}\n  A --> B\n`);

    const guide = screen.getByRole("link", { name: "guide" });
    expect(guide.closest(".mermaid-block")).toBeNull();
    expect(guide.closest("pre")).toBeNull();
    fireEvent.click(guide);
    expect(openFile).toHaveBeenCalledWith("src/guide.md");
  });

  // @covers AC-UI-MERMAID-RENDERING-002.6
  it("keeps task-scoped error suppression for an unrecoverable Mermaid block", async () => {
    const content = [MERMAID_FENCE, FLOWCHART_LR, "  A -->", "```"].join("\n");
    const first = render(<MemoizedMarkdown content={content} taskId="mermaid-task-context" />);
    await flushRender();
    expect(mockToast).toHaveBeenCalledOnce();
    first.unmount();

    render(<MemoizedMarkdown content={content} taskId="mermaid-task-context" />);
    await flushRender();

    expect(mockToast).toHaveBeenCalledOnce();
  });

  // @covers AC-UI-MERMAID-RENDERING-002.6
  it("does not publish an older diagram or suffix after its render resolves late", async () => {
    const oldMessage = [MERMAID_FENCE, FLOWCHART_LR, "  OldNode --> B", "```Old prose."].join("\n");
    const newMessage = [MERMAID_FENCE, FLOWCHART_LR, "  NewNode --> B", "```New prose."].join("\n");
    const view = render(<MemoizedMarkdown content={oldMessage} taskId="mermaid-stale-task" />);
    await flushRender();

    expect(renderControl.oldRenderStarted).toHaveBeenCalledOnce();
    expect(renderInputs).toEqual([`${FLOWCHART_LR}\n  OldNode --> B\n`]);

    view.rerender(<MemoizedMarkdown content={newMessage} taskId="mermaid-stale-task" />);
    await flushRender();

    expect(view.container.querySelector("svg")?.getAttribute("data-diagram")).toBe("new");
    expect(screen.getByText("New prose.", { exact: true })).toBeTruthy();
    expect(renderInputs).toEqual([
      `${FLOWCHART_LR}\n  OldNode --> B\n`,
      `${FLOWCHART_LR}\n  NewNode --> B\n`,
    ]);

    await act(async () => {
      renderControl.resolveOld?.({ svg: '<svg data-diagram="old"></svg>' });
      await Promise.resolve();
      await Promise.resolve();
      await Promise.resolve();
    });

    expect(view.container.querySelector("svg")?.getAttribute("data-diagram")).toBe("new");
    expect(screen.getByText("New prose.", { exact: true })).toBeTruthy();
    expect(screen.queryByText("Old prose.", { exact: true })).toBeNull();
  });
});

describe("chat Markdown Mermaid review regressions", () => {
  it("keeps document reference definitions available to recovered prose", async () => {
    const content = [
      "[guide]: https://example.com/guide",
      "",
      MERMAID_FENCE,
      FLOWCHART_LR,
      "  A --> B",
      "```See [guide].",
    ].join("\n");

    render(
      <MemoizedMarkdown content={content} taskId="mermaid-reference-task" onOpenFile={vi.fn()} />,
    );
    await flushRender();

    expect(screen.getByRole("link", { name: "guide" }).getAttribute("href")).toBe(
      "https://example.com/guide",
    );
  });

  it("shows the current source after a newer render fails while retaining the prior SVG", async () => {
    const validMessage = [MERMAID_FENCE, FLOWCHART_LR, "  A --> B", "```"].join("\n");
    const invalidMessage = [MERMAID_FENCE, FLOWCHART_LR, "  A -->", "```"].join("\n");
    const view = render(
      <MemoizedMarkdown content={validMessage} taskId="mermaid-current-code-task" />,
    );
    await flushRender();

    expect(view.container.querySelector("svg")).not.toBeNull();
    view.rerender(<MemoizedMarkdown content={invalidMessage} taskId="mermaid-current-code-task" />);
    await flushRender();

    expect(view.container.querySelector("svg")).not.toBeNull();
    expect(mockToast).not.toHaveBeenCalled();
    fireEvent.click(screen.getByTitle("Show code"));
    const codeView = view.container.querySelector(".mermaid-block pre code");
    expect(codeView?.textContent).toContain("A -->");
    expect(codeView?.textContent).not.toContain("A --> B");
  });
});
