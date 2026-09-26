import { cleanup, fireEvent, render } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { createElement } from "react";
import {
  computeStyle,
  containerPaddingBox,
  type ContainerPaddingBox,
  MessageHistorySearch,
} from "./message-history-search";

afterEach(cleanup);

describe("containerPaddingBox", () => {
  it("recovers the padding box by subtracting the border via clientLeft/clientTop", () => {
    // A container with an 800x600 border box at (100, 80) and a 1px border on
    // every side -- clientLeft/clientTop/clientWidth/clientHeight are exactly
    // the values a real element with `border: 1px solid` would report (jsdom
    // always reports 0 for these, which is why this arithmetic is extracted
    // into a pure function rather than asserted against a rendered node).
    const container = {
      getBoundingClientRect: () => new DOMRect(100, 80, 800, 600),
      clientLeft: 1,
      clientTop: 1,
      clientWidth: 798,
      clientHeight: 598,
    };

    expect(containerPaddingBox(container)).toEqual({
      left: 101,
      width: 798,
      bottom: 80 + 1 + 598,
    });
  });

  it("matches the border box when there is no border (clientLeft/clientTop are 0)", () => {
    const container = {
      getBoundingClientRect: () => new DOMRect(0, 0, 400, 300),
      clientLeft: 0,
      clientTop: 0,
      clientWidth: 400,
      clientHeight: 300,
    };

    expect(containerPaddingBox(container)).toEqual({ left: 0, width: 400, bottom: 300 });
  });
});

describe("computeStyle", () => {
  it("returns null without an anchor rect", () => {
    expect(computeStyle(null, null)).toBeNull();
  });

  it("positions relative to the viewport when there is no containing block (document.body target)", () => {
    const anchorRect = new DOMRect(40, 500, 300, 32);

    const style = computeStyle(anchorRect, null);

    expect(style).toMatchObject({
      position: "fixed",
      left: 40,
      bottom: window.innerHeight - 500 + 8,
    });
  });

  it("clamps left to the 8px minimum when the anchor sits at the viewport edge", () => {
    const anchorRect = new DOMRect(-20, 500, 300, 32);

    const style = computeStyle(anchorRect, null);

    expect(style?.left).toBe(8);
  });

  it("positions relative to a transformed containing block (Quick Chat's DialogContent) instead of the viewport", () => {
    // DialogContent is centered with a permanent translate, so it does not
    // sit at the viewport origin -- its rect has its own left/top/bottom.
    const containerBox: ContainerPaddingBox = { left: 100, width: 800, bottom: 680 };
    // Anchor (the composer) is inside that container, still given in
    // viewport coordinates by getBoundingClientRect().
    const anchorRect = new DOMRect(140, 600, 300, 32);

    const style = computeStyle(anchorRect, containerBox);

    // Origin-relative: anchorRect.left - containerBox.left, and
    // containerBox.bottom - anchorRect.top + 8, not viewport-relative.
    expect(style).toMatchObject({
      position: "fixed",
      left: 140 - 100,
      bottom: 680 - 600 + 8,
    });
  });

  it("matches the viewport-relative formula when containerRect is null (regression guard for the document.body case)", () => {
    const anchorRect = new DOMRect(40, 500, 300, 32);

    const viewportRelative = computeStyle(anchorRect, null);
    const explicitViewportOrigin = computeStyle(anchorRect, {
      left: 0,
      width: window.innerWidth,
      bottom: window.innerHeight,
    });

    expect(viewportRelative).toEqual(explicitViewportOrigin);
  });
});

describe("MessageHistorySearch Escape fallback close (F11)", () => {
  const history = [{ content: "hello world", entityReferences: [] }];
  const anchorRect = new DOMRect(40, 500, 300, 32);

  it("closes on Escape when the input itself is the target (baseline, via its own onKeyDown)", () => {
    const onClose = vi.fn();
    const { getByTestId } = render(
      createElement(MessageHistorySearch, {
        history,
        anchorRect,
        container: document.body,
        onClose,
        onSelect: vi.fn(),
      }),
    );

    fireEvent.keyDown(getByTestId("history-search-input"), { key: "Escape" });

    expect(onClose).toHaveBeenCalledTimes(1);
  });

  /**
   * F11: tabbing out of the reverse-search input leaves the overlay's only
   * focusable element unfocused. On Quick Chat, `container` is the Dialog's
   * own `[data-slot="dialog-content"]` -- unique to this one composer's
   * modal -- so the fallback listener broadens its ownership check to that
   * ancestor instead of just the popup's own small div, and still closes the
   * overlay for an Escape whose target has moved elsewhere in the same
   * dialog.
   */
  it("closes the overlay when focus tabs out to elsewhere in the same modal (container is not document.body)", () => {
    const dialogContent = document.createElement("div");
    dialogContent.setAttribute("data-slot", "dialog-content");
    document.body.appendChild(dialogContent);
    const somethingElseInTheDialog = document.createElement("button");
    dialogContent.appendChild(somethingElseInTheDialog);
    const onClose = vi.fn();

    render(
      createElement(MessageHistorySearch, {
        history,
        anchorRect,
        container: dialogContent,
        onClose,
        onSelect: vi.fn(),
      }),
    );

    fireEvent.keyDown(somethingElseInTheDialog, { key: "Escape", bubbles: true, cancelable: true });

    expect(onClose).toHaveBeenCalledTimes(1);

    document.body.removeChild(dialogContent);
  });

  /**
   * On the main task chat panel, `container` is `document.body` -- claiming
   * that whole scope would race a second, unrelated overlay mounted
   * elsewhere on the page (the same cross-composer bug F10 fixed for
   * suggestion popups). The fallback must stay precise to this popup's own
   * container in that case, even though it means the literal "tab out of the
   * input" repro is not fully closed on that one surface.
   */
  it("does not react to an Escape targeting an unrelated element when container is document.body", () => {
    const unrelated = document.createElement("button");
    document.body.appendChild(unrelated);
    const onClose = vi.fn();

    render(
      createElement(MessageHistorySearch, {
        history,
        anchorRect,
        container: document.body,
        onClose,
        onSelect: vi.fn(),
      }),
    );

    fireEvent.keyDown(unrelated, { key: "Escape", bubbles: true, cancelable: true });

    expect(onClose).not.toHaveBeenCalled();

    document.body.removeChild(unrelated);
  });
});
