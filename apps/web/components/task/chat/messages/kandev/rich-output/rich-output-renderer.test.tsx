import { act, cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { KandevRendererProps } from "../types";
import { RichOutputRenderer } from "./rich-output-renderer";
const ACCEPTED_CONTENT = "Accepted file content";
const PREVIEW_TOGGLE = "rich-output-file-preview-toggle";
const transport = vi.hoisted(() => ({ read: vi.fn(), client: {} }));
vi.mock("@/lib/ws/connection", () => ({ getWebSocketClient: () => transport.client }));
vi.mock("@/lib/ws/workspace-files", () => ({ requestFileContent: transport.read }));
function RendererHarness(props: KandevRendererProps) {
  return RichOutputRenderer(props);
}
afterEach(() => {
  cleanup();
  transport.read.mockReset();
});
function composition() {
  return {
    version: 2,
    catalog_version: 1,
    title: "Stable result",
    spec: {
      root: "report",
      elements: {
        report: {
          type: "Stack",
          props: { direction: "horizontal" },
          children: ["explanation", "file"],
        },
        explanation: { type: "Group", props: { title: "Context" }, children: ["note"] },
        note: { type: "Text", props: { text: "<script> is plain text" }, children: [] },
        file: { type: "FilePreview", props: { path: "reports/result.txt" }, children: [] },
      },
    },
  };
}
describe("RichOutputRenderer with the actual catalog", () => {
  // @covers AC-AGENTS-AGENT-RICH-OUTPUT-002.1, AC-AGENTS-AGENT-RICH-OUTPUT-002.11
  it("renders grouping and plain text without initially reading files", () => {
    render(
      <RendererHarness args={composition()} result={undefined} status="complete" sessionId="s1" />,
    );
    expect(screen.getByText("Context")).toBeTruthy();
    expect(screen.getByText("<script> is plain text")).toBeTruthy();
    expect(document.querySelector("script")).toBeNull();
    expect(transport.read).not.toHaveBeenCalled();
  });
  // @covers AC-AGENTS-AGENT-RICH-OUTPUT-001.11, AC-AGENTS-AGENT-RICH-OUTPUT-002.7
  it("retains preview state through new persisted-input objects with the same target", async () => {
    transport.read.mockResolvedValue({ content: ACCEPTED_CONTENT, is_binary: false });
    const view = render(
      <RendererHarness args={composition()} result={undefined} status="complete" sessionId="s1" />,
    );
    fireEvent.click(screen.getByTestId(PREVIEW_TOGGLE));
    expect(await screen.findByText(ACCEPTED_CONTENT)).toBeTruthy();
    view.rerender(
      <RendererHarness
        args={{ ...composition(), title: "New title" }}
        result={undefined}
        status="complete"
        sessionId="s1"
      />,
    );
    expect(screen.getByTestId(PREVIEW_TOGGLE).getAttribute("aria-expanded")).toBe("true");
    expect(screen.getByText(ACCEPTED_CONTENT)).toBeTruthy();
    expect(transport.read).toHaveBeenCalledTimes(1);
  });
});
// @covers AC-AGENTS-AGENT-RICH-OUTPUT-001.13
it("shows a localized unavailable state when a native component fails", async () => {
  const args = composition();
  transport.read.mockResolvedValue({ content: null, is_binary: false });
  const errors = vi.spyOn(console, "error").mockImplementation(() => undefined);
  try {
    render(<RendererHarness args={args} result={undefined} status="complete" sessionId="s1" />);
    fireEvent.click(screen.getByTestId(PREVIEW_TOGGLE));
    expect(await screen.findByText("This presentation is unavailable.")).toBeTruthy();
    expect(screen.getByText("Context")).toBeTruthy();
  } finally {
    errors.mockRestore();
  }
});

// @covers AC-AGENTS-AGENT-RICH-OUTPUT-002.8
it("reflows the mounted catalog at the phone breakpoint without resetting a preview", async () => {
  const originalWidth = window.innerWidth;
  const listeners = new Set<() => void>();
  const media = vi.spyOn(window, "matchMedia").mockImplementation(
    (query) =>
      ({
        media: query,
        matches: query.includes("pointer: fine"),
        onchange: null,
        addEventListener: (_event: string, listener: () => void) => listeners.add(listener),
        removeEventListener: (_event: string, listener: () => void) => listeners.delete(listener),
        addListener: () => undefined,
        removeListener: () => undefined,
        dispatchEvent: () => true,
      }) as unknown as MediaQueryList,
  );
  const resize = (width: number) =>
    act(() => {
      Object.defineProperty(window, "innerWidth", { configurable: true, value: width });
      listeners.forEach((listener) => listener());
    });
  try {
    resize(1024);
    transport.read.mockResolvedValue({ content: ACCEPTED_CONTENT, is_binary: false });
    render(
      <RendererHarness args={composition()} result={undefined} status="complete" sessionId="s1" />,
    );
    const stack = screen.getByTestId("rich-output-stack");
    const toggle = screen.getByTestId(PREVIEW_TOGGLE);
    expect(stack.className).toContain("flex-row");
    fireEvent.click(toggle);
    await screen.findByText(ACCEPTED_CONTENT);
    resize(767);
    expect(stack.className).toContain("flex-col");
    expect(screen.getByTestId(PREVIEW_TOGGLE)).toBe(toggle);
    expect(toggle.getAttribute("aria-expanded")).toBe("true");
    resize(768);
    expect(stack.className).toContain("flex-row");
    expect(screen.getByText(ACCEPTED_CONTENT)).toBeTruthy();
    expect(transport.read).toHaveBeenCalledTimes(1);
  } finally {
    cleanup();
    Object.defineProperty(window, "innerWidth", { configurable: true, value: originalWidth });
    media.mockRestore();
  }
});
