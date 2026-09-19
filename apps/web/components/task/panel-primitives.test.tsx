import { createRef } from "react";
import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it } from "vitest";
import {
  PanelFooterBar,
  PanelHeaderBar,
  PanelHeaderBarSplit,
  PanelHeaderOverflowMenu,
  shouldUsePanelHeaderOverflow,
} from "./panel-primitives";

afterEach(cleanup);

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
          data-testid="split-header"
          left={<span>Title</span>}
          right={<button type="button">Action</button>}
          rightWhenOverflow={<button type="button">Primary action</button>}
          overflow={<button type="button">More actions</button>}
          overflowAt={520}
        />
        <PanelFooterBar data-testid="panel-footer">Footer</PanelFooterBar>
      </>,
    );

    expect(screen.getByTestId("split-header").textContent).toContain("TitleAction");
    expect(screen.getByTestId("split-header").textContent).not.toContain("Primary action");
    expect(screen.getByTestId("panel-footer").className).not.toContain("h-[30px]");
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
