import { act, cleanup, fireEvent, render } from "@testing-library/react";
import { createElement, createRef } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { ReviewFile } from "./types";
import {
  captureReviewScrollAnchor,
  restoreReviewScrollAnchor,
  useReviewScrollAnchor,
} from "./use-review-scroll-anchor";

const FILE_KEY = "src%2Fa.ts";
const FILE_PATH = "src/a.ts";
const ADDITION_LINE_TYPE = "change-addition";
const READING_LINE_TEXT = "the reading line";
const STABLE_AFTER_TEXT = "stable after";

function setRect(element: Element, top: number, bottom = top + 20) {
  Object.defineProperty(element, "getBoundingClientRect", {
    configurable: true,
    value: () => ({ top, bottom, left: 0, right: 300, width: 300, height: bottom - top }),
  });
}

function addLine(
  section: HTMLElement,
  line: number,
  side: string,
  top: number,
  content = `line-${line}`,
) {
  const existingHost = section.querySelector("diffs-container");
  const host = existingHost ?? document.createElement("diffs-container");
  const shadow = host.shadowRoot ?? host.attachShadow({ mode: "open" });
  const lineElement = document.createElement("div");
  lineElement.dataset.line = String(line);
  lineElement.dataset.lineType = side;
  lineElement.textContent = content;
  setRect(lineElement, top);
  shadow.append(lineElement);
  if (!existingHost) section.append(host);
  return lineElement;
}

function rootWithSection(fileKey = FILE_KEY) {
  const root = document.createElement("div");
  setRect(root, 100, 500);
  Object.defineProperties(root, {
    scrollHeight: { configurable: true, value: 900 },
    scrollWidth: { configurable: true, value: 500 },
    clientHeight: { configurable: true, value: 400 },
    clientWidth: { configurable: true, value: 300 },
  });
  const section = document.createElement("section");
  section.dataset.reviewFileKey = fileKey;
  root.append(section);
  return { root, section };
}

function file(diff: string, displayScopeKey = "scope-1"): ReviewFile {
  return {
    path: FILE_PATH,
    diff,
    status: "modified",
    additions: 1,
    deletions: 1,
    staged: false,
    source: "uncommitted",
    display_scope_key: displayScopeKey,
  };
}

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

describe("review scroll anchor line snapshots", () => {
  it("captures a visible file, line side, and offset, then restores that line", () => {
    const { root, section } = rootWithSection();
    const line = addLine(section, 40, ADDITION_LINE_TYPE, 220);
    const anchor = captureReviewScrollAnchor(root);
    expect(anchor).toMatchObject({
      fileKey: FILE_KEY,
      line: 40,
      side: "addition",
      offset: 120,
    });

    root.scrollTop = 100;
    setRect(line, 350);
    expect(restoreReviewScrollAnchor(root, anchor!)).toBe("line");
    expect(root.scrollTop).toBe(230);
  });

  it("uses the nearest surviving line on the same side when the exact line disappeared", () => {
    const { root, section } = rootWithSection();
    addLine(section, 18, "change-deletion", 290);
    addLine(section, 23, "change-deletion", 340);
    const result = restoreReviewScrollAnchor(root, {
      fileKey: FILE_KEY,
      line: 20,
      side: "deletion",
      offset: 50,
      scrollTop: 0,
      scrollLeft: 4,
    });

    expect(result).toBe("line");
    expect(root.scrollTop).toBe(140);
    expect(root.scrollLeft).toBe(4);
  });

  it("restores the same content after lines are inserted and deleted above it", () => {
    const { root, section } = rootWithSection();
    addLine(section, 59, ADDITION_LINE_TYPE, 700, "stable before");
    addLine(section, 60, ADDITION_LINE_TYPE, 220, READING_LINE_TEXT);
    addLine(section, 61, ADDITION_LINE_TYPE, 720, STABLE_AFTER_TEXT);
    const anchor = captureReviewScrollAnchor(root)!;
    expect(anchor).toMatchObject({ lineText: READING_LINE_TEXT });

    section.replaceChildren();
    addLine(section, 66, ADDITION_LINE_TYPE, 350, READING_LINE_TEXT);
    addLine(section, 67, ADDITION_LINE_TYPE, 730, STABLE_AFTER_TEXT);
    root.scrollTop = 100;

    expect(restoreReviewScrollAnchor(root, anchor)).toBe("line");
    expect(root.scrollTop).toBe(230);
  });

  it("uses surviving nearby content when the anchored line was removed", () => {
    const { root, section } = rootWithSection();
    addLine(section, 18, ADDITION_LINE_TYPE, 280, "stable before");
    addLine(section, 30, ADDITION_LINE_TYPE, 300, "nearby changed content");
    addLine(section, 31, ADDITION_LINE_TYPE, 320, STABLE_AFTER_TEXT);
    const result = restoreReviewScrollAnchor(root, {
      fileKey: FILE_KEY,
      line: 30,
      side: "addition",
      lineText: "removed reading line",
      beforeLines: ["stable before"],
      afterLines: [STABLE_AFTER_TEXT],
      offset: 50,
      scrollTop: 0,
      scrollLeft: 4,
    });

    expect(result).toBe("line");
    expect(root.scrollTop).toBe(170);
    expect(root.scrollLeft).toBe(4);
  });

  it("clamps prior offsets when the anchored file was removed", () => {
    const { root } = rootWithSection("src%2Fb.ts");
    root.scrollTop = 20;
    expect(
      restoreReviewScrollAnchor(root, {
        fileKey: FILE_KEY,
        line: 40,
        side: "context",
        offset: 20,
        scrollTop: 850,
        scrollLeft: 900,
      }),
    ).toBe("clamped");
    expect(root.scrollTop).toBe(500);
    expect(root.scrollLeft).toBe(200);
  });
});

describe("review scroll anchor section snapshots", () => {
  it("captures the list section when the active renderer owns its internal line scroll", () => {
    const { root, section } = rootWithSection();
    setRect(section, 240);
    const anchor = captureReviewScrollAnchor(root);
    expect(anchor).toMatchObject({ fileKey: "src%2Fa.ts", side: "section", offset: 140 });

    root.scrollTop = 50;
    setRect(section, 320);
    expect(restoreReviewScrollAnchor(root, anchor!)).toBe("section");
    expect(root.scrollTop).toBe(130);
  });
});

describe("review scroll anchor input cancellation", () => {
  it("cancels a scheduled restoration after user input", () => {
    const frames = new Map<number, FrameRequestCallback>();
    let nextFrame = 0;
    vi.stubGlobal("requestAnimationFrame", (callback: FrameRequestCallback) => {
      const id = ++nextFrame;
      frames.set(id, callback);
      return id;
    });
    vi.stubGlobal("cancelAnimationFrame", (id: number) => frames.delete(id));

    const rootRef = createRef<HTMLDivElement>();
    const suppression = { current: false };
    function Probe({ patch }: { patch: string }) {
      const { handleUserInput } = useReviewScrollAnchor({
        rootRef,
        files: [file(patch)],
        sessionId: "session-1",
        sourceKey: "uncommitted",
        suppressAutoMark: suppression,
      });
      return createElement(
        "div",
        {
          ref: (element: HTMLDivElement | null) => {
            rootRef.current = element;
            if (!element) return;
            setRect(element, 100, 500);
            Object.defineProperties(element, {
              scrollHeight: { configurable: true, value: 900 },
              scrollWidth: { configurable: true, value: 500 },
              clientHeight: { configurable: true, value: 400 },
              clientWidth: { configurable: true, value: 300 },
            });
          },
          onWheel: handleUserInput,
          "data-testid": "root",
        },
        createElement(
          "section",
          { "data-review-file-key": FILE_KEY },
          createElement(DiffLine, { top: patch === "old" ? 220 : 350 }),
        ),
      );
    }
    function DiffLine({ top }: { top: number }) {
      const ref = (host: HTMLElement | null) => {
        if (!host || host.shadowRoot) return;
        const shadow = host.attachShadow({ mode: "open" });
        const line = document.createElement("div");
        line.dataset.line = "40";
        line.dataset.lineType = "change-addition";
        setRect(line, top);
        shadow.append(line);
      };
      return createElement("diffs-container", { ref });
    }

    const view = render(createElement(Probe, { patch: "old" }));
    const root = view.getByTestId("root");
    act(() => view.rerender(createElement(Probe, { patch: "new" })));
    expect(frames.size).toBe(1);

    fireEvent.wheel(root);
    expect(frames.size).toBe(0);
    act(() => {
      for (const [id, callback] of frames) {
        callback(0);
        frames.delete(id);
      }
    });
    expect(root.scrollTop).toBe(0);
  });
});
