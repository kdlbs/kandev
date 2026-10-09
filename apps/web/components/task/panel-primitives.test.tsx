import { createRef } from "react";
import { cleanup, render, screen, waitFor } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import {
  PanelFooterBar,
  PanelHeaderBar,
  PanelHeaderBarSplit,
  PanelHeaderOverflowMenu,
  shouldUsePanelHeaderOverflow,
} from "./panel-primitives";

afterEach(cleanup);

const SPLIT_TITLE = "Split title";
const SPLIT_ACTION = "Split action";
const RESIZE_TITLE = "Resize title";
const RESIZE_ACTION = "Resize action";
const MORE_ACTIONS = "More actions";
const SPLIT_HEADER_TEST_ID = "split-header";
const PANEL_FOOTER_TEST_ID = "panel-footer";

describe("panel header primitives", () => {
  it("passes standard div attributes and refs through the shared header shell", () => {
    const ref = createRef<HTMLDivElement>();

    render(
      <PanelHeaderBar ref={ref} data-testid="panel-header" aria-label="Panel actions">
        Header
      </PanelHeaderBar>,
    );

    const header = screen.getByTestId("panel-header");
    expect(header.getAttribute("aria-label")).toBe("Panel actions");
    expect(header.textContent).toContain("Header");
    expect(ref.current).toBe(header);
    expect(header.className).toContain("h-[1.875rem]");
    expect(header.className).toContain("[@media(max-width:47.999rem)]:h-12");
    expect(header.className).toContain("[@media(pointer:coarse)]:h-12");
    expect(header.className).toContain("[@media(max-width:47.999rem)]:[&_button]:min-h-11");
    expect(header.className).toContain("[@media(pointer:coarse)]:[&_a]:min-w-11");
  });

  it.each([
    [239, true],
    [480, true],
    [520, false],
  ])("resolves width-aware overflow at %ipx", (width, expected) => {
    expect(shouldUsePanelHeaderOverflow(width, 520)).toBe(expected);
  });

  it("keeps split actions in the shared header and leaves footer geometry separate", () => {
    render(
      <>
        <PanelHeaderBarSplit
          data-testid={SPLIT_HEADER_TEST_ID}
          left={<span>{SPLIT_TITLE}</span>}
          right={<button type="button">{SPLIT_ACTION}</button>}
          rightWhenOverflow={<button type="button">Primary action</button>}
          overflow={<button type="button">{MORE_ACTIONS}</button>}
          overflowAt={520}
        />
        <PanelFooterBar data-testid={PANEL_FOOTER_TEST_ID}>Footer</PanelFooterBar>
      </>,
    );

    expect(screen.getByTestId(SPLIT_HEADER_TEST_ID).textContent).toContain(SPLIT_TITLE);
    expect(screen.getByTestId(SPLIT_HEADER_TEST_ID).textContent).toContain(SPLIT_ACTION);
    expect(screen.getByTestId(SPLIT_HEADER_TEST_ID).textContent).not.toContain("Primary action");
    expect(screen.getByTestId(PANEL_FOOTER_TEST_ID).className).not.toContain("h-[30px]");
  });

  it("exposes a keyboard-addressable overflow trigger for secondary actions", () => {
    render(
      <PanelHeaderOverflowMenu label="More panel actions">
        <button type="button">Open editor</button>
      </PanelHeaderOverflowMenu>,
    );

    const trigger = screen.getByRole("button", { name: "More panel actions" });
    expect(trigger.getAttribute("data-testid")).toBe("panel-header-overflow");
    expect(trigger.className).toContain("max-md:size-11");
  });
});

describe("panel header overflow state", () => {
  it("clears a stale overflow state when overflow detection is disabled", async () => {
    class ResizeObserverMock {
      constructor(private readonly callback: ResizeObserverCallback) {}

      observe(target: Element) {
        Object.defineProperty(target, "getBoundingClientRect", {
          configurable: true,
          value: () => ({ width: 240 }),
        });
        this.callback([{ target } as ResizeObserverEntry], this as unknown as ResizeObserver);
      }

      disconnect() {}
    }

    vi.stubGlobal("ResizeObserver", ResizeObserverMock);
    const view = render(
      <PanelHeaderBarSplit
        data-testid={SPLIT_HEADER_TEST_ID}
        left={<span>{RESIZE_TITLE}</span>}
        right={<button type="button">{RESIZE_ACTION}</button>}
        overflow={<button type="button">{MORE_ACTIONS}</button>}
        overflowAt={520}
      />,
    );

    await waitFor(() =>
      expect(screen.getByTestId(SPLIT_HEADER_TEST_ID).getAttribute("data-panel-overflow")).toBe(
        "true",
      ),
    );

    view.rerender(
      <PanelHeaderBarSplit
        data-testid={SPLIT_HEADER_TEST_ID}
        left={<span>{RESIZE_TITLE}</span>}
        right={<button type="button">{RESIZE_ACTION}</button>}
      />,
    );

    await waitFor(() =>
      expect(
        screen.getByTestId(SPLIT_HEADER_TEST_ID).getAttribute("data-panel-overflow"),
      ).toBeNull(),
    );
    expect(
      screen
        .getByRole("button", { name: RESIZE_ACTION })
        .parentElement?.className.split(/\s+/)
        .includes("hidden"),
    ).toBe(false);
    vi.unstubAllGlobals();
  });
});
