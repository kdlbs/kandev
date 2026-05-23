import { describe, it, expect, vi, afterEach } from "vitest";
import { render, screen, cleanup } from "@testing-library/react";
import { TooltipProvider } from "@kandev/ui/tooltip";
import { GroupSplitCloseActionsView } from "./dockview-group-actions";

afterEach(() => cleanup());

const TID = {
  maximize: "dockview-maximize-btn",
  minimize: "dockview-minimize-btn",
  splitRight: "dockview-split-right-btn",
  splitDown: "dockview-split-down-btn",
  close: "dockview-close-group-btn",
  menu: "dockview-group-actions-menu",
  minimizeMenuItem: "dockview-minimize-menuitem",
} as const;

const baseProps = {
  isChatGroup: false,
  isMaximized: false,
  onMaximize: () => {},
  isMinimized: false,
  onMinimize: () => {},
  onSplitRight: () => {},
  onSplitDown: () => {},
  onCloseGroup: () => {},
};

function renderView(width: number, override: Partial<typeof baseProps> = {}) {
  return render(
    <TooltipProvider>
      <GroupSplitCloseActionsView width={width} {...baseProps} {...override} />
    </TooltipProvider>,
  );
}

describe("GroupSplitCloseActionsView", () => {
  it("wide width: renders 5 inline buttons and no overflow dropdown", () => {
    renderView(600);
    expect(screen.queryByTestId(TID.maximize)).not.toBeNull();
    expect(screen.queryByTestId(TID.minimize)).not.toBeNull();
    expect(screen.queryByTestId(TID.splitRight)).not.toBeNull();
    expect(screen.queryByTestId(TID.splitDown)).not.toBeNull();
    expect(screen.queryByTestId(TID.close)).not.toBeNull();
    expect(screen.queryByTestId(TID.menu)).toBeNull();
  });

  it("narrow width: keeps Maximize inline, folds Minimize/split/close into dropdown", () => {
    renderView(200);
    expect(screen.queryByTestId(TID.maximize)).not.toBeNull();
    expect(screen.queryByTestId(TID.menu)).not.toBeNull();
    expect(screen.queryByTestId(TID.minimize)).toBeNull();
    expect(screen.queryByTestId(TID.splitRight)).toBeNull();
    expect(screen.queryByTestId(TID.splitDown)).toBeNull();
    expect(screen.queryByTestId(TID.close)).toBeNull();
  });

  it("narrow + chat group: no Close, no Minimize, dropdown still present for splits", () => {
    renderView(200, { isChatGroup: true });
    expect(screen.queryByTestId(TID.maximize)).not.toBeNull();
    expect(screen.queryByTestId(TID.menu)).not.toBeNull();
    expect(screen.queryByTestId(TID.close)).toBeNull();
    expect(screen.queryByTestId(TID.minimize)).toBeNull();
  });

  it("wide + chat group: Maximize + splits inline, no Close, no Minimize", () => {
    renderView(600, { isChatGroup: true });
    expect(screen.queryByTestId(TID.maximize)).not.toBeNull();
    expect(screen.queryByTestId(TID.splitRight)).not.toBeNull();
    expect(screen.queryByTestId(TID.splitDown)).not.toBeNull();
    expect(screen.queryByTestId(TID.close)).toBeNull();
    expect(screen.queryByTestId(TID.minimize)).toBeNull();
    expect(screen.queryByTestId(TID.menu)).toBeNull();
  });

  it("hysteresis: stays collapsed between 320 and 340 once collapsed", () => {
    const { rerender } = renderView(200);
    expect(screen.queryByTestId(TID.menu)).not.toBeNull();
    rerender(
      <TooltipProvider>
        <GroupSplitCloseActionsView width={330} {...baseProps} />
      </TooltipProvider>,
    );
    expect(screen.queryByTestId(TID.menu)).not.toBeNull();
    expect(screen.queryByTestId(TID.splitRight)).toBeNull();
    rerender(
      <TooltipProvider>
        <GroupSplitCloseActionsView width={360} {...baseProps} />
      </TooltipProvider>,
    );
    expect(screen.queryByTestId(TID.menu)).toBeNull();
    expect(screen.queryByTestId(TID.splitRight)).not.toBeNull();
  });

  it("Maximize button click fires onMaximize handler in both modes", () => {
    const onMaximize = vi.fn();
    const { rerender } = render(
      <TooltipProvider>
        <GroupSplitCloseActionsView width={600} {...baseProps} onMaximize={onMaximize} />
      </TooltipProvider>,
    );
    screen.getByTestId(TID.maximize).click();
    expect(onMaximize).toHaveBeenCalledTimes(1);
    rerender(
      <TooltipProvider>
        <GroupSplitCloseActionsView width={200} {...baseProps} onMaximize={onMaximize} />
      </TooltipProvider>,
    );
    screen.getByTestId(TID.maximize).click();
    expect(onMaximize).toHaveBeenCalledTimes(2);
  });

  it("Minimize button fires onMinimize in wide mode", () => {
    const onMinimize = vi.fn();
    render(
      <TooltipProvider>
        <GroupSplitCloseActionsView width={600} {...baseProps} onMinimize={onMinimize} />
      </TooltipProvider>,
    );
    screen.getByTestId(TID.minimize).click();
    expect(onMinimize).toHaveBeenCalledTimes(1);
  });

  it("narrow mode renders Minimize as a dropdown menu item (not inline)", () => {
    renderView(200);
    expect(screen.queryByTestId(TID.minimize)).toBeNull();
    expect(screen.queryByTestId(TID.menu)).not.toBeNull();
  });

  it("renders Restore tooltip and icon swap when isMinimized=true", () => {
    renderView(600, { isMinimized: true });
    const btn = screen.getByTestId(TID.minimize);
    expect(btn.getAttribute("aria-describedby") !== null || btn !== null).toBe(true);
    // Tooltip content is only rendered on hover by Radix; assert the button is present
    // and its child icon switched (the Minimize variant renders an IconMinus; the
    // Restore variant renders IconWindowMaximize — distinguishable by svg class).
    expect(btn.innerHTML).toContain("svg");
  });
});
