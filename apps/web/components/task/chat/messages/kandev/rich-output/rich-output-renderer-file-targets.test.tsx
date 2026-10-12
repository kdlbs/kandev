import { act, cleanup, fireEvent, render, screen, within } from "@testing-library/react";
import { afterEach, beforeEach, expect, it, vi } from "vitest";
import type { KandevRendererProps } from "../types";
import { RichOutputRenderer } from "./rich-output-renderer";
import type { RichOutputFileBlock } from "./types";

const transport = vi.hoisted(() => ({ read: vi.fn(), client: {} }));

vi.mock("@/lib/ws/connection", () => ({ getWebSocketClient: () => transport.client }));
vi.mock("@/lib/ws/workspace-files", () => ({ requestFileContent: transport.read }));

const PREVIEW_TOGGLE = "rich-output-file-preview-toggle";
const EXPANDED_ATTRIBUTE = "aria-expanded";
const PREVIEW_FAILURE = "Preview unavailable.";
const PREVIEW_LOADING = "Loading preview";

type Target = { sessionId: string; path: string; repo?: string };
const initial: Target = { sessionId: "session-a", path: "a.txt", repo: "alpha" };

function propsFor(
  target: Target,
  metadata: Partial<Pick<RichOutputFileBlock, "title" | "caption">> = {},
): KandevRendererProps {
  return {
    status: "complete",
    sessionId: target.sessionId,
    result: undefined,
    args: {
      version: 1,
      title: "Artifacts",
      blocks: [
        { type: "file", path: target.path, repo: target.repo, title: "Report", ...metadata },
      ],
    },
  };
}

function RendererHarness(props: KandevRendererProps) {
  return RichOutputRenderer(props);
}

function contentFor(target: Target) {
  return `${target.sessionId}/${target.repo}/${target.path} content`;
}

function toggle() {
  return screen.getByTestId(PREVIEW_TOGGLE);
}

async function togglePreview() {
  await act(async () => fireEvent.click(toggle()));
}

function deferredPreview() {
  let resolve!: (value: { content: string; is_binary: boolean }) => void;
  let reject!: (reason: Error) => void;
  const promise = new Promise<{ content: string; is_binary: boolean }>(
    (resolvePromise, rejectPromise) => {
      resolve = resolvePromise;
      reject = rejectPromise;
    },
  );
  return { promise, resolve, reject };
}

async function settleOld(request: ReturnType<typeof deferredPreview>, outcome: string) {
  await act(async () => {
    if (outcome === "success") request.resolve({ content: "Old target content", is_binary: false });
    else request.reject(new Error("Old target failed"));
    await request.promise.catch(() => undefined);
  });
}

beforeEach(() => {
  transport.read.mockReset();
  transport.read.mockImplementation(async (_client, sessionId, path, repo) => ({
    content: contentFor({ sessionId, path, repo }),
    is_binary: false,
  }));
});
afterEach(cleanup);

// @covers AC-AGENTS-AGENT-RICH-OUTPUT-001.9
it.each([
  { field: "path", target: { ...initial, path: "b.txt" } },
  { field: "repository", target: { ...initial, repo: "beta" } },
  { field: "session", target: { ...initial, sessionId: "session-b" } },
])("collapses a loaded preview when its $field changes", async ({ target }) => {
  const view = render(<RendererHarness {...propsFor(initial)} />);
  await togglePreview();
  expect(screen.getByText(contentFor(initial))).toBeTruthy();
  expect(toggle().getAttribute(EXPANDED_ATTRIBUTE)).toBe("true");

  view.rerender(<RendererHarness {...propsFor(target)} />);

  expect(toggle().getAttribute(EXPANDED_ATTRIBUTE)).toBe("false");
  expect(screen.queryByText(contentFor(initial))).toBeNull();
  expect(screen.queryByText(PREVIEW_FAILURE)).toBeNull();
  expect(screen.queryByText(PREVIEW_LOADING)).toBeNull();
  expect(transport.read).toHaveBeenCalledOnce();
  await togglePreview();
  expect(transport.read).toHaveBeenLastCalledWith(
    transport.client,
    target.sessionId,
    target.path,
    target.repo,
  );
  expect(screen.getByText(contentFor(target))).toBeTruthy();
  expect(transport.read).toHaveBeenCalledTimes(2);
});

// @covers AC-AGENTS-AGENT-RICH-OUTPUT-001.11
it("loads lazily and reuses successful preview content", async () => {
  render(<RendererHarness {...propsFor(initial)} />);
  expect(toggle().getAttribute(EXPANDED_ATTRIBUTE)).toBe("false");
  expect(transport.read).not.toHaveBeenCalled();
  await togglePreview();
  expect(screen.getByText(contentFor(initial))).toBeTruthy();
  await togglePreview();
  expect(screen.queryByText(contentFor(initial))).toBeNull();
  await togglePreview();
  expect(screen.getByText(contentFor(initial))).toBeTruthy();
  expect(transport.read).toHaveBeenCalledOnce();
});

// @covers AC-AGENTS-AGENT-RICH-OUTPUT-001.9
it("keeps collapsed descriptor replacement lazy until the new target is expanded", async () => {
  const target = { ...initial, path: "b.txt" };
  const view = render(<RendererHarness {...propsFor(initial)} />);
  view.rerender(<RendererHarness {...propsFor(target)} />);
  expect(transport.read).not.toHaveBeenCalled();
  expect(toggle().getAttribute(EXPANDED_ATTRIBUTE)).toBe("false");
  await togglePreview();
  expect(transport.read).toHaveBeenCalledExactlyOnceWith(
    transport.client,
    target.sessionId,
    target.path,
    target.repo,
  );
  expect(screen.getByText(contentFor(target))).toBeTruthy();
});

// @covers AC-AGENTS-AGENT-RICH-OUTPUT-001.10
it.each(["success", "error"])(
  "discards pending old %s while the replacement is collapsed",
  async (outcome) => {
    const request = deferredPreview();
    transport.read.mockReturnValueOnce(request.promise);
    const target = { ...initial, path: "b.txt" };
    const view = render(<RendererHarness {...propsFor(initial)} />);
    await togglePreview();
    try {
      expect(screen.getByText(PREVIEW_LOADING)).toBeTruthy();
      view.rerender(<RendererHarness {...propsFor(target)} />);
      expect(toggle().getAttribute(EXPANDED_ATTRIBUTE)).toBe("false");
      expect(screen.queryByText(PREVIEW_LOADING)).toBeNull();
      expect(screen.queryByText(PREVIEW_FAILURE)).toBeNull();
      expect(transport.read).toHaveBeenCalledOnce();
      await settleOld(request, outcome);
      expect(toggle().getAttribute(EXPANDED_ATTRIBUTE)).toBe("false");
      expect(screen.queryByText("Old target content")).toBeNull();
      expect(screen.queryByText(PREVIEW_FAILURE)).toBeNull();
      await togglePreview();
      expect(screen.getByText(contentFor(target))).toBeTruthy();
      expect(transport.read).toHaveBeenCalledTimes(2);
    } finally {
      await settleOld(request, outcome);
    }
  },
);

// @covers AC-AGENTS-AGENT-RICH-OUTPUT-001.10
it.each(["success", "error"])("keeps the new preview after late old %s", async (outcome) => {
  const request = deferredPreview();
  transport.read.mockReturnValueOnce(request.promise);
  const target = { ...initial, path: "b.txt" };
  const view = render(<RendererHarness {...propsFor(initial)} />);
  await togglePreview();
  try {
    view.rerender(<RendererHarness {...propsFor(target)} />);
    expect(toggle().getAttribute(EXPANDED_ATTRIBUTE)).toBe("false");
    await togglePreview();
    expect(screen.getByText(contentFor(target))).toBeTruthy();
    await settleOld(request, outcome);
    expect(screen.getByText(contentFor(target))).toBeTruthy();
    expect(toggle().getAttribute(EXPANDED_ATTRIBUTE)).toBe("true");
    expect(screen.queryByText("Old target content")).toBeNull();
    expect(screen.queryByText(PREVIEW_FAILURE)).toBeNull();
    expect(transport.read).toHaveBeenCalledTimes(2);
  } finally {
    await settleOld(request, outcome);
  }
});

// @covers AC-AGENTS-AGENT-RICH-OUTPUT-001.11
it("preserves cached disclosure through same-target rerenders and metadata changes", async () => {
  const props = propsFor(initial);
  const view = render(<RendererHarness {...props} />);
  await togglePreview();
  view.rerender(<RendererHarness {...props} />);
  view.rerender(<RendererHarness {...propsFor(initial)} />);
  view.rerender(
    <RendererHarness {...propsFor(initial, { title: "Updated report", caption: "New caption" })} />,
  );
  expect(screen.getByText("Updated report")).toBeTruthy();
  expect(screen.getByText("New caption")).toBeTruthy();
  expect(screen.getByText(contentFor(initial))).toBeTruthy();
  expect(toggle().getAttribute(EXPANDED_ATTRIBUTE)).toBe("true");
  await togglePreview();
  await togglePreview();
  expect(screen.getByText(contentFor(initial))).toBeTruthy();
  expect(transport.read).toHaveBeenCalledOnce();
});

// @covers AC-AGENTS-AGENT-RICH-OUTPUT-001.11
it("retains a same-target error and retries only after explicit re-expansion", async () => {
  transport.read.mockRejectedValueOnce(new Error("Current read failed"));
  const view = render(<RendererHarness {...propsFor(initial)} />);
  await togglePreview();
  expect(screen.getByText(PREVIEW_FAILURE)).toBeTruthy();
  view.rerender(<RendererHarness {...propsFor(initial, { caption: "Updated caption" })} />);
  expect(screen.getByText(PREVIEW_FAILURE)).toBeTruthy();
  expect(toggle().getAttribute(EXPANDED_ATTRIBUTE)).toBe("true");
  expect(transport.read).toHaveBeenCalledOnce();
  await togglePreview();
  expect(transport.read).toHaveBeenCalledOnce();
  await togglePreview();
  expect(screen.getByText(contentFor(initial))).toBeTruthy();
  expect(screen.queryByText(PREVIEW_FAILURE)).toBeNull();
  expect(transport.read).toHaveBeenCalledTimes(2);
});

// @covers AC-AGENTS-AGENT-RICH-OUTPUT-001.9
it("clears a previous target error without reading the replacement", async () => {
  transport.read.mockRejectedValueOnce(new Error("Current read failed"));
  const view = render(<RendererHarness {...propsFor(initial)} />);
  await togglePreview();
  expect(screen.getByText(PREVIEW_FAILURE)).toBeTruthy();
  const target = { ...initial, path: "b.txt" };
  view.rerender(<RendererHarness {...propsFor(target)} />);
  expect(toggle().getAttribute(EXPANDED_ATTRIBUTE)).toBe("false");
  expect(screen.queryByText(PREVIEW_FAILURE)).toBeNull();
  expect(transport.read).toHaveBeenCalledOnce();
  await togglePreview();
  expect(screen.getByText(contentFor(target))).toBeTruthy();
  expect(transport.read).toHaveBeenCalledTimes(2);
});

// @covers AC-AGENTS-AGENT-RICH-OUTPUT-001.12
it("keeps duplicate-target cards independent when only the first is replaced", async () => {
  const block = { type: "file", path: initial.path, repo: initial.repo, title: "Report" };
  const props = {
    ...propsFor(initial),
    args: { version: 1, title: "Artifacts", blocks: [block, block] },
  };
  const view = render(<RendererHarness {...props} />);
  for (const card of screen.getAllByTestId("rich-output-file")) {
    await act(async () => fireEvent.click(within(card).getByTestId(PREVIEW_TOGGLE)));
  }
  expect(transport.read).toHaveBeenCalledTimes(2);
  view.rerender(
    <RendererHarness
      {...props}
      args={{ ...props.args, blocks: [{ ...block, path: "b.txt" }, block] }}
    />,
  );
  const [first, second] = screen.getAllByTestId("rich-output-file");
  expect(within(first).getByTestId(PREVIEW_TOGGLE).getAttribute(EXPANDED_ATTRIBUTE)).toBe("false");
  expect(within(second).getByText(contentFor(initial))).toBeTruthy();
  expect(within(second).getByTestId(PREVIEW_TOGGLE).getAttribute(EXPANDED_ATTRIBUTE)).toBe("true");
  expect(transport.read).toHaveBeenCalledTimes(2);
  await act(async () => fireEvent.click(within(first).getByTestId(PREVIEW_TOGGLE)));
  expect(within(first).getByText(contentFor({ ...initial, path: "b.txt" }))).toBeTruthy();
  expect(within(second).getByText(contentFor(initial))).toBeTruthy();
  expect(within(second).getByTestId(PREVIEW_TOGGLE).getAttribute(EXPANDED_ATTRIBUTE)).toBe("true");
  expect(transport.read).toHaveBeenCalledTimes(3);
});
