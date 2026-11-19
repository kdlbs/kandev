import { describe, expect, it } from "vitest";
import {
  bridgeModeForTool,
  captureNumbers,
  closeCommentStep,
  isScreenshotLimitReached,
  nextEscapeStep,
  recaptureRect,
} from "./preview-annotation";

describe("bridgeModeForTool", () => {
  it("maps tools to bridge modes", () => {
    expect(bridgeModeForTool("interact", false)).toBeNull();
    expect(bridgeModeForTool("text", false)).toBe("text");
    expect(bridgeModeForTool("element", false)).toBe("element");
    expect(bridgeModeForTool("region", false)).toBe("screenshot");
  });

  it("passes input through when the region tool is at the screenshot limit", () => {
    expect(bridgeModeForTool("region", true)).toBeNull();
    expect(bridgeModeForTool("text", true)).toBe("text");
  });
});

describe("nextEscapeStep", () => {
  it("closes an open surface first", () => {
    expect(nextEscapeStep({ surfaceOpen: true, tool: "region" })).toBe("close-surface");
    expect(nextEscapeStep({ surfaceOpen: true, tool: "interact" })).toBe("close-surface");
  });

  it("then switches a capture tool to Interact", () => {
    expect(nextEscapeStep({ surfaceOpen: false, tool: "text" })).toBe("interact");
    expect(nextEscapeStep({ surfaceOpen: false, tool: "element" })).toBe("interact");
    expect(nextEscapeStep({ surfaceOpen: false, tool: "region" })).toBe("interact");
  });

  it("then exits annotation mode", () => {
    expect(nextEscapeStep({ surfaceOpen: false, tool: "interact" })).toBe("exit");
  });
});

describe("isScreenshotLimitReached", () => {
  it("counts saved and unsaved screenshots together", () => {
    expect(isScreenshotLimitReached(9, 0)).toBe(false);
    expect(isScreenshotLimitReached(6, 4)).toBe(true);
    expect(isScreenshotLimitReached(0, 10)).toBe(true);
  });
});

describe("recaptureRect", () => {
  const original = {
    x: 10,
    y: 20,
    width: 100,
    height: 50,
    document_x: 10,
    document_y: 520,
    scroll_x: 0,
    scroll_y: 500,
    viewport_width: 800,
    viewport_height: 600,
    device_pixel_ratio: 2,
    scroll_container_selector: "div#list",
    container_scroll_x: 0,
    container_scroll_y: 40,
  };

  it("anchors the new rectangle at the page's current scroll positions", () => {
    const frame = {
      scrollX: 0,
      scrollY: 300,
      innerWidth: 820,
      innerHeight: 610,
      containerScroll: () => ({ x: 0, y: 90 }),
    };

    expect(recaptureRect(original, { x: 30, y: 40, width: 60, height: 70 }, frame)).toEqual({
      ...original,
      x: 30,
      y: 40,
      width: 60,
      height: 70,
      document_x: 30,
      document_y: 340,
      scroll_x: 0,
      scroll_y: 300,
      viewport_width: 820,
      viewport_height: 610,
      container_scroll_x: 0,
      container_scroll_y: 90,
    });
  });

  it("replaces the previous anchor with the new one, or drops it", () => {
    const frame = {
      scrollX: 0,
      scrollY: 0,
      innerWidth: 800,
      innerHeight: 600,
      containerScroll: () => null,
    };
    const anchored = {
      ...original,
      anchor_selector: "section#old",
      anchor_offset_x: 1,
      anchor_offset_y: 2,
      anchor_width: 30,
      anchor_height: 40,
    };
    const anchor = {
      anchor_selector: "section#card",
      anchor_offset_x: 5,
      anchor_offset_y: 6,
      anchor_width: 300,
      anchor_height: 200,
    };
    const moved = { x: 1, y: 2, width: 10, height: 10 };
    expect(recaptureRect(anchored, moved, frame, { anchor, container: null })).toMatchObject(
      anchor,
    );

    const unanchored = recaptureRect(anchored, moved, frame, null);
    expect(unanchored.anchor_selector).toBeUndefined();
    expect(unanchored.anchor_width).toBeUndefined();
  });

  it("keeps the stored container offsets when the container is gone", () => {
    const frame = {
      scrollX: 0,
      scrollY: 0,
      innerWidth: 800,
      innerHeight: 600,
      containerScroll: () => null,
    };
    const next = recaptureRect(original, { x: 1, y: 2, width: 10, height: 10 }, frame);
    expect(next.container_scroll_y).toBe(40);
  });
});

describe("captureNumbers", () => {
  it("numbers captures per route in list order", () => {
    const numbers = captureNumbers([
      { id: "a", route: "/" },
      { id: "b", route: "/pricing" },
      { id: "c", route: "/" },
      { id: "d", route: "/pricing" },
    ]);
    expect(Object.fromEntries(numbers)).toEqual({ a: 1, b: 1, c: 2, d: 2 });
  });
});

describe("closeCommentStep", () => {
  it("discards an empty new capture and keeps a typed one", () => {
    expect(closeCommentStep({ draftComment: "", editing: false })).toBe("discard-draft");
    expect(closeCommentStep({ draftComment: "   ", editing: false })).toBe("discard-draft");
    expect(closeCommentStep({ draftComment: "bigger", editing: false })).toBe("keep-draft");
  });

  it("closes a saved item's editor without saving", () => {
    expect(closeCommentStep({ draftComment: null, editing: true })).toBe("close-editor");
  });

  it("does nothing when no comment is open", () => {
    expect(closeCommentStep({ draftComment: null, editing: false })).toBe("none");
  });
});

describe("recaptureRect placement", () => {
  const original = {
    x: 10,
    y: 20,
    width: 100,
    height: 50,
    scroll_container_selector: "div#list",
    container_scroll_x: 0,
    container_scroll_y: 40,
  };

  it("takes the scroll container the page reports for the new position", () => {
    const frame = {
      scrollX: 0,
      scrollY: 0,
      innerWidth: 800,
      innerHeight: 600,
      containerScroll: () => ({ x: 0, y: 90 }),
    };
    const moved = { x: 1, y: 2, width: 10, height: 10 };

    const outside = recaptureRect(original, moved, frame, { anchor: null, container: null });
    expect(outside.scroll_container_selector).toBeUndefined();
    expect(outside.container_scroll_y).toBeUndefined();

    const container = {
      scroll_container_selector: "div#panel",
      container_scroll_x: 3,
      container_scroll_y: 4,
    };
    expect(recaptureRect(original, moved, frame, { anchor: null, container })).toMatchObject(
      container,
    );
  });
});
