import { act, cleanup, fireEvent, render, screen, within } from "@testing-library/react";
import { afterEach, beforeEach, expect, it, vi } from "vitest";
import type { KandevRendererProps } from "../types";
import { RichOutputRenderer } from "./rich-output-renderer";
import type { FilePreviewProps } from "@/lib/rich-output/catalog";

const transport = vi.hoisted(() => ({ read: vi.fn(), client: {} }));

vi.mock("@/lib/ws/connection", () => ({ getWebSocketClient: () => transport.client }));
vi.mock("@/lib/ws/workspace-files", () => ({ requestFileContent: transport.read }));

const PREVIEW_TOGGLE = "rich-output-file-preview-toggle";
const EXPANDED_ATTRIBUTE = "aria-expanded";
const PREVIEW_FAILURE = "Preview unavailable.";
const PREVIEW_LOADING = "Loading preview";
const OLD_CONTENT = "Old target content";
const EDITED_CONTENT = "Updated workspace content";

type Target = { sessionId: string; path: string; repo?: string };
const initial: Target = { sessionId: "session-a", path: "a.txt", repo: "alpha" };

function propsFor(
  target: Target,
  metadata: Partial<Pick<FilePreviewProps, "title" | "caption">> = {},
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

function propsForVersion(target: Target, version: 1 | 2): KandevRendererProps {
  if (version === 1) return propsFor(target);
  return {
    status: "complete",
    sessionId: target.sessionId,
    result: undefined,
    args: {
      version: 2,
      catalog_version: 1,
      title: "Artifacts",
      spec: {
        root: "report",
        elements: {
          report: {
            type: "FilePreview",
            props: { path: target.path, repo: target.repo },
            children: [],
          },
        },
      },
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
    if (outcome === "success") request.resolve({ content: OLD_CONTENT, is_binary: false });
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

// @covers AC-AGENTS-IMAGE-PREVIEW-DEFAULTS-001.1, AC-AGENTS-IMAGE-PREVIEW-DEFAULTS-001.5
// @covers AC-AGENTS-IMAGE-PREVIEW-EXPANSION-001.1, AC-AGENTS-IMAGE-PREVIEW-EXPANSION-001.3
it.each([1, 2])(
  "loads and expands an image in version %i without another read or file navigation",
  async (version) => {
    const target = { ...initial, path: "screenshot.PNG" };
    const props = propsFor(target);
    if (version === 2) {
      props.args = {
        version: 2,
        catalog_version: 1,
        title: "Artifacts",
        spec: {
          root: "image",
          elements: {
            image: {
              type: "FilePreview",
              props: { path: target.path, repo: target.repo },
              children: [],
            },
          },
        },
      };
    }
    transport.read.mockResolvedValue({ content: "aW1hZ2U=", is_binary: true });
    const onOpenFile = vi.fn();
    render(<RendererHarness {...props} onOpenFile={onOpenFile} />);
    expect(toggle().getAttribute(EXPANDED_ATTRIBUTE)).toBe("true");
    const triggerName = version === 1 ? "Open Report" : "Open screenshot.PNG";
    const trigger = await screen.findByRole("button", { name: triggerName });
    const image = within(trigger).getByRole("presentation");
    expect(image.getAttribute("src")).toBe("data:image/png;base64,aW1hZ2U=");
    expect(transport.read).toHaveBeenCalledWith(
      transport.client,
      target.sessionId,
      target.path,
      target.repo,
    );
    await act(async () => fireEvent.click(trigger));
    const dialog = screen.getByRole("dialog", { name: "Image preview" });
    expect(within(dialog).getByRole("img").getAttribute("src")).toBe(image.getAttribute("src"));
    await act(async () =>
      fireEvent.click(within(dialog).getByRole("button", { name: "Actual size" })),
    );
    expect(within(dialog).getByRole("img").getAttribute("src")).toBe(image.getAttribute("src"));
    expect(transport.read).toHaveBeenCalledOnce();
    await act(async () => fireEvent.click(within(dialog).getByRole("button", { name: "Close" })));
    expect(screen.queryByRole("dialog")).toBeNull();
    expect(toggle().getAttribute(EXPANDED_ATTRIBUTE)).toBe("true");
    expect(onOpenFile).not.toHaveBeenCalled();
    await togglePreview();
    expect(screen.queryByRole("button", { name: triggerName })).toBeNull();
    await togglePreview();
    expect(
      within(screen.getByRole("button", { name: triggerName }))
        .getByRole("presentation")
        .getAttribute("src"),
    ).toBe(image.getAttribute("src"));
    expect(transport.read).toHaveBeenCalledOnce();
    await act(async () => fireEvent.click(screen.getByTestId("rich-output-file-open")));
    expect(onOpenFile).toHaveBeenCalledExactlyOnceWith(target.path, target.repo);
  },
);

// @covers AC-AGENTS-IMAGE-PREVIEW-DEFAULTS-001.2
it.each(["notes.txt", "archive.zip", "unknown"])("keeps %s collapsed without reading", (path) => {
  render(<RendererHarness {...propsFor({ ...initial, path })} />);
  expect(toggle().getAttribute(EXPANDED_ATTRIBUTE)).toBe("false");
  expect(transport.read).not.toHaveBeenCalled();
});

// @covers AC-AGENTS-IMAGE-PREVIEW-DEFAULTS-001.4
it.each([
  { field: "path", target: { ...initial, path: "new.png" } },
  { field: "repository", target: { ...initial, path: "old.png", repo: "beta" } },
  { field: "session", target: { ...initial, path: "old.png", sessionId: "session-b" } },
])("loads only the new image target when its $field changes", async ({ target }) => {
  const original = { ...initial, path: "old.png" };
  const view = render(<RendererHarness {...propsFor(original)} />);
  await screen.findByText(contentFor(original));
  view.rerender(<RendererHarness {...propsFor(target)} />);
  await screen.findByText(contentFor(target));
  expect(screen.queryByText(contentFor(original))).toBeNull();
  expect(toggle().getAttribute(EXPANDED_ATTRIBUTE)).toBe("true");
  expect(transport.read).toHaveBeenLastCalledWith(
    transport.client,
    target.sessionId,
    target.path,
    target.repo,
  );
  expect(transport.read).toHaveBeenCalledTimes(2);
});

// @covers AC-AGENTS-IMAGE-PREVIEW-DEFAULTS-001.4
it.each(["success", "failure"])(
  "ignores old image %s after an image/text replacement",
  async (outcome) => {
    const pending = deferredPreview();
    const image = { ...initial, path: "old.png" };
    transport.read.mockReturnValueOnce(pending.promise);
    const view = render(<RendererHarness {...propsFor(image)} />);
    try {
      expect(screen.getByText(PREVIEW_LOADING)).toBeTruthy();
      const text = { ...initial, path: "new.txt" };
      view.rerender(<RendererHarness {...propsFor(text)} />);
      expect(toggle().getAttribute(EXPANDED_ATTRIBUTE)).toBe("false");
      expect(transport.read).toHaveBeenCalledOnce();
      const replacement = { ...initial, path: "new.png" };
      view.rerender(<RendererHarness {...propsFor(replacement)} />);
      await screen.findByText(contentFor(replacement));
      await settleOld(pending, outcome);
      expect(screen.getByText(contentFor(replacement))).toBeTruthy();
      expect(screen.queryByText(OLD_CONTENT)).toBeNull();
      expect(screen.queryByText(PREVIEW_FAILURE)).toBeNull();
      expect(transport.read).toHaveBeenCalledTimes(2);
    } finally {
      await settleOld(pending, outcome);
    }
  },
);

// @covers AC-AGENTS-IMAGE-PREVIEW-DEFAULTS-001.5
it("keeps an automatic image read failure local until explicit reopening", async () => {
  const target = { ...initial, path: "failed.png" };
  transport.read.mockRejectedValueOnce(new Error("Read failed"));
  const view = render(<RendererHarness {...propsFor(target)} />);
  await screen.findByText(PREVIEW_FAILURE);
  view.rerender(<RendererHarness {...propsFor(target, { caption: "Updated" })} />);
  expect(transport.read).toHaveBeenCalledOnce();
  await togglePreview();
  expect(transport.read).toHaveBeenCalledOnce();
  await togglePreview();
  await screen.findByText(contentFor(target));
  expect(transport.read).toHaveBeenCalledTimes(2);
});

// @covers AC-AGENTS-AGENT-RICH-OUTPUT-001.9
it.each([
  { field: "path", target: { ...initial, path: "b.txt" } },
  { field: "repository", target: { ...initial, repo: "beta" } },
  { field: "session", target: { ...initial, sessionId: "session-b" } },
])("collapses a text preview when its $field changes", async ({ target }) => {
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

// @covers AC-AGENTS-AGENT-RICH-OUTPUT-001.11, AC-AGENTS-AGENT-RICH-OUTPUT-001.14
it.each([1, 2] as const)(
  "reads an edited file on reopening a mounted version %i preview",
  async (version) => {
    let currentContent = contentFor(initial);
    transport.read.mockImplementation(async () => ({ content: currentContent, is_binary: false }));
    render(<RendererHarness {...propsForVersion(initial, version)} />);
    expect(toggle().getAttribute(EXPANDED_ATTRIBUTE)).toBe("false");
    expect(transport.read).not.toHaveBeenCalled();

    await togglePreview();
    expect(screen.getByText(contentFor(initial))).toBeTruthy();
    expect(transport.read).toHaveBeenCalledOnce();

    await togglePreview();
    expect(screen.queryByText(contentFor(initial))).toBeNull();
    expect(transport.read).toHaveBeenCalledOnce();
    currentContent = EDITED_CONTENT;

    await togglePreview();
    expect(screen.getByText(EDITED_CONTENT)).toBeTruthy();
    expect(transport.read).toHaveBeenCalledTimes(2);
    expect(transport.read).toHaveBeenLastCalledWith(
      transport.client,
      initial.sessionId,
      initial.path,
      initial.repo,
    );
  },
);

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
      expect(screen.queryByText(OLD_CONTENT)).toBeNull();
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
    expect(screen.queryByText(OLD_CONTENT)).toBeNull();
    expect(screen.queryByText(PREVIEW_FAILURE)).toBeNull();
    expect(transport.read).toHaveBeenCalledTimes(2);
  } finally {
    await settleOld(request, outcome);
  }
});

// @covers AC-AGENTS-AGENT-RICH-OUTPUT-001.11
it("preserves disclosure through same-target rerenders without extra reads", async () => {
  const props = propsFor(initial);
  const view = render(<RendererHarness {...props} />);
  await togglePreview();
  expect(transport.read).toHaveBeenCalledOnce();
  view.rerender(<RendererHarness {...props} />);
  view.rerender(<RendererHarness {...propsFor(initial)} />);
  view.rerender(
    <RendererHarness {...propsFor(initial, { title: "Updated report", caption: "New caption" })} />,
  );
  expect(screen.getByText("Updated report")).toBeTruthy();
  expect(screen.getByText("New caption")).toBeTruthy();
  expect(screen.getByText(contentFor(initial))).toBeTruthy();
  expect(toggle().getAttribute(EXPANDED_ATTRIBUTE)).toBe("true");
  expect(transport.read).toHaveBeenCalledOnce();
  await togglePreview();
  expect(transport.read).toHaveBeenCalledOnce();
  await togglePreview();
  expect(screen.getByText(contentFor(initial))).toBeTruthy();
  expect(transport.read).toHaveBeenCalledTimes(2);
});

// @covers AC-AGENTS-AGENT-RICH-OUTPUT-001.11
it.each(["success", "error"])(
  "keeps the reopened result after a late same-target %s",
  async (outcome) => {
    const previousRead = deferredPreview();
    transport.read
      .mockReturnValueOnce(previousRead.promise)
      .mockResolvedValueOnce({ content: EDITED_CONTENT, is_binary: false });
    render(<RendererHarness {...propsFor(initial)} />);
    try {
      await togglePreview();
      expect(transport.read).toHaveBeenCalledOnce();
      await togglePreview();
      await togglePreview();
      expect(screen.getByText(EDITED_CONTENT)).toBeTruthy();
      expect(transport.read).toHaveBeenCalledTimes(2);
      await settleOld(previousRead, outcome);
      expect(screen.getByText(EDITED_CONTENT)).toBeTruthy();
      expect(screen.queryByText(OLD_CONTENT)).toBeNull();
      expect(screen.queryByText(PREVIEW_FAILURE)).toBeNull();
    } finally {
      await settleOld(previousRead, outcome);
    }
  },
);

// @covers AC-AGENTS-AGENT-RICH-OUTPUT-001.11
it("shows an unavailable read and fetches restored contents after reopening", async () => {
  transport.read
    .mockResolvedValueOnce({ content: "Existing workspace content", is_binary: false })
    .mockRejectedValueOnce(new Error("File is missing"))
    .mockResolvedValueOnce({ content: "Restored workspace content", is_binary: false });
  render(<RendererHarness {...propsFor(initial)} />);

  await togglePreview();
  expect(screen.getByText("Existing workspace content")).toBeTruthy();
  expect(transport.read).toHaveBeenCalledOnce();
  await togglePreview();
  expect(transport.read).toHaveBeenCalledOnce();
  await togglePreview();
  expect(screen.getByText(PREVIEW_FAILURE)).toBeTruthy();
  expect(transport.read).toHaveBeenCalledTimes(2);
  await togglePreview();
  await togglePreview();
  expect(screen.getByText("Restored workspace content")).toBeTruthy();
  expect(screen.queryByText(PREVIEW_FAILURE)).toBeNull();
  expect(transport.read).toHaveBeenCalledTimes(3);
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

// @covers AC-AGENTS-IMAGE-PREVIEW-EXPANSION-001.4
it.each([
  { field: "path", target: { ...initial, path: "new.png" } },
  { field: "repository", target: { ...initial, path: "old.png", repo: "beta" } },
  { field: "session", target: { ...initial, path: "old.png", sessionId: "session-b" } },
])("dismisses an open image when its $field changes", async ({ target }) => {
  transport.read.mockResolvedValueOnce({ content: "b2xk", is_binary: true });
  transport.read.mockResolvedValue({ content: "bmV3", is_binary: true });
  const view = render(<RendererHarness {...propsFor({ ...initial, path: "old.png" })} />);
  const trigger = await screen.findByRole("button", { name: "Open Report" });
  await act(async () => fireEvent.click(trigger));
  expect(within(screen.getByRole("dialog")).getByRole("img").getAttribute("src")).toBe(
    "data:image/png;base64,b2xk",
  );
  view.rerender(<RendererHarness {...propsFor(target)} />);
  expect(screen.queryByRole("dialog")).toBeNull();
  const nextTrigger = await screen.findByRole("button", { name: "Open Report" });
  await act(async () => fireEvent.click(nextTrigger));
  expect(within(screen.getByRole("dialog")).getByRole("img").getAttribute("src")).toBe(
    "data:image/png;base64,bmV3",
  );
  view.unmount();
  expect(screen.queryByRole("dialog")).toBeNull();
});
