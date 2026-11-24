import { readFileSync } from "node:fs";
import path from "node:path";
import { afterEach, beforeAll, beforeEach, vi } from "vitest";

const SCRIPT_PATH = path.resolve(
  __dirname,
  "../../backend/internal/agentctl/server/api/scripts/inspector.js",
);
const SOURCE = "kandev-inspector";
export const VERSION = 3;

export const SET_CAPTURE_MODE = "set-capture-mode";
export const CAPTURE_MODE_CHANGED = "capture-mode-changed";
export const REGION_LAYOUT = "region-layout";
export const SET_ANNOTATION_ACTIVE = "set-annotation-active";
export const PROJECT_MARKERS = "project-markers";

type Sent = { source: string; version: number; type: string; payload: Record<string, unknown> };

export const postMessage = vi.fn<(message: Sent) => void>();
let frames: FrameRequestCallback[] = [];

export function flushFrames() {
  const pending = frames;
  frames = [];
  pending.forEach((callback) => callback(0));
}

export function command(type: string, payload: Record<string, unknown> = {}) {
  window.dispatchEvent(
    new MessageEvent("message", { data: { source: SOURCE, version: VERSION, type, payload } }),
  );
}

export function sentOfType(type: string) {
  return postMessage.mock.calls.map(([message]) => message).filter((m) => m.type === type);
}

export function rect(left: number, top: number, width: number, height: number): DOMRect {
  return {
    left,
    top,
    width,
    height,
    right: left + width,
    bottom: top + height,
    x: left,
    y: top,
    toJSON: () => ({}),
  } as DOMRect;
}

export function pointer(type: string, x: number, y: number) {
  document.body.dispatchEvent(
    new PointerEvent(type, { clientX: x, clientY: y, pointerId: 1, bubbles: true }),
  );
}

/** Loads the page script once into this test file's jsdom and resets the page between tests. */
export function useInspectorPage() {
  beforeAll(() => {
    Object.defineProperty(window, "parent", { value: { postMessage }, configurable: true });
    window.requestAnimationFrame = (callback) => {
      frames.push(callback);
      return frames.length;
    };
    vi.spyOn(Range.prototype, "getClientRects").mockReturnValue([
      rect(20, 30, 80, 16),
    ] as unknown as DOMRectList);
    new Function(readFileSync(SCRIPT_PATH, "utf8"))();
  });

  beforeEach(() => {
    document.body.innerHTML =
      '<p id="copy">Lorem ipsum dolor</p><button id="buy">Buy</button>' +
      '<div id="list" style="overflow: auto"><span id="row">Row</span></div>';
    postMessage.mockClear();
    frames = [];
  });

  afterEach(() => {
    command(SET_CAPTURE_MODE, { mode: null });
    command(SET_ANNOTATION_ACTIVE, { active: false });
    command(PROJECT_MARKERS, { markers: [] });
    window.getSelection()?.removeAllRanges();
    flushFrames();
  });
}
