import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { RegionOverlay } from "./region-overlay";

afterEach(cleanup);

const items = [
  { index: 0, rect: { x: 1, y: 2, width: 30, height: 20 } },
  { index: 2, rect: { x: 40, y: 10, width: 20, height: 20 }, title: "Too tight" },
];

describe("RegionOverlay", () => {
  it("numbers regions by their index and renders no handles by default", () => {
    render(<RegionOverlay items={items} selectedIndex={null} draft={null} testId="region" />);

    const regions = screen.getAllByTestId("region");
    expect(regions.map((region) => region.textContent)).toEqual(["1", "3"]);
    expect(regions[1].getAttribute("title")).toBe("Too tight");
    expect(regions[0].getAttribute("data-pending")).toBeNull();
    expect(screen.queryAllByTestId("region-handle")).toHaveLength(0);
  });

  it("draws pending regions dashed", () => {
    render(
      <RegionOverlay
        items={items}
        selectedIndex={null}
        draft={null}
        testId="region"
        pendingIndexes={new Set([2])}
      />,
    );

    const pending = screen.getAllByTestId("region")[1];
    expect(pending.getAttribute("data-pending")).toBe("true");
    expect(pending.className).toContain("border-dashed");
  });

  it("reports move and corner handle presses with the region index", () => {
    const onHandlePointerDown = vi.fn();
    render(
      <RegionOverlay
        items={items}
        selectedIndex={null}
        draft={null}
        testId="region"
        onHandlePointerDown={onHandlePointerDown}
      />,
    );

    const second = screen.getAllByTestId("region")[1];
    fireEvent.pointerDown(second);
    const corner = second.querySelector('[data-region-handle="se"]');
    expect(corner).not.toBeNull();
    fireEvent.pointerDown(corner as Element);

    expect(onHandlePointerDown).toHaveBeenNthCalledWith(1, 2, "move", expect.anything());
    expect(onHandlePointerDown).toHaveBeenNthCalledWith(2, 2, "se", expect.anything());
    expect(onHandlePointerDown).toHaveBeenCalledTimes(2);
  });
});
