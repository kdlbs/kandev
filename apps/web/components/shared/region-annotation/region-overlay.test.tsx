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

  it("marks busy regions with an animated border", () => {
    render(
      <RegionOverlay
        items={items}
        selectedIndex={null}
        draft={null}
        testId="region"
        busyIndexes={new Set([0])}
      />,
    );

    const [busy, idle] = screen.getAllByTestId("region");
    expect(busy.getAttribute("data-busy")).toBe("true");
    expect(busy.querySelector(".kandev-region-busy")).not.toBeNull();
    expect(idle.getAttribute("data-busy")).toBeNull();
    expect(idle.querySelector(".kandev-region-busy")).toBeNull();
  });

  it("shows host numbers and reports badge clicks without starting a move", () => {
    const onSelect = vi.fn();
    const onHandlePointerDown = vi.fn();
    render(
      <RegionOverlay
        items={[{ ...items[0], number: 4 }, items[1]]}
        selectedIndex={null}
        draft={null}
        testId="region"
        onSelect={onSelect}
        onHandlePointerDown={onHandlePointerDown}
      />,
    );

    const badge = screen.getAllByTestId("region")[0].querySelector("[data-region-badge]");
    expect(badge?.textContent).toBe("4");
    fireEvent.pointerDown(badge as Element);
    fireEvent.click(badge as Element);

    expect(onSelect).toHaveBeenCalledWith(0);
    expect(onHandlePointerDown).not.toHaveBeenCalled();
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

const CAPTURE_TOGGLE = "region-capture-toggle";
const CAPTURE_IMAGE = "region-capture-image";

const labels = {
  drifted: "The page layout changed since this capture.",
  showCapture: "Show capture",
  hideCapture: "Hide capture",
};

describe("RegionOverlay alignment controls", () => {
  const captured = [
    { index: 0, rect: { x: 1, y: 2, width: 30, height: 20 }, imageUrl: "blob:one" },
    { index: 1, rect: { x: 40, y: 10, width: 20, height: 20 } },
  ];

  it("draws a drifted region as a warning with an explanation", () => {
    render(
      <RegionOverlay
        items={captured}
        selectedIndex={null}
        draft={null}
        testId="region"
        driftedIndexes={new Set([0])}
        labels={labels}
      />,
    );
    const [drifted, aligned] = screen.getAllByTestId("region");
    expect(drifted.getAttribute("data-drifted")).toBe("true");
    expect(drifted.className).toContain("border-amber-500");
    expect(drifted.className).toContain("border-dashed");
    expect(screen.getByRole("button", { name: labels.drifted })).toBeTruthy();
    expect(aligned.getAttribute("data-drifted")).toBeNull();
    expect(screen.getAllByTestId("region-drift-warning")).toHaveLength(1);
  });

  it("shows and hides the saved capture over a region with an image", () => {
    render(
      <RegionOverlay
        items={captured}
        selectedIndex={null}
        draft={null}
        testId="region"
        labels={labels}
      />,
    );
    const toggle = screen.getByRole("button", { name: labels.showCapture });
    expect(screen.getAllByTestId(CAPTURE_TOGGLE)).toHaveLength(1);
    expect(toggle.getAttribute("aria-pressed")).toBe("false");

    fireEvent.click(toggle);
    const shown = screen.getByRole("button", { name: labels.hideCapture });
    expect(shown.getAttribute("aria-pressed")).toBe("true");
    expect(screen.getByTestId(CAPTURE_IMAGE).getAttribute("src")).toBe("blob:one");

    fireEvent.click(shown);
    expect(screen.queryByTestId(CAPTURE_IMAGE)).toBeNull();
  });

  it("shows the capture while the button is held", () => {
    render(<RegionOverlay items={captured} selectedIndex={null} draft={null} labels={labels} />);
    const toggle = screen.getByTestId(CAPTURE_TOGGLE);
    fireEvent.pointerDown(toggle);
    expect(screen.getByTestId(CAPTURE_IMAGE)).toBeTruthy();
    fireEvent.pointerUp(toggle);
    expect(screen.queryByTestId(CAPTURE_IMAGE)).toBeNull();
  });

  it("hides the capture when the region starts moving", () => {
    render(
      <RegionOverlay
        items={captured}
        selectedIndex={null}
        draft={null}
        testId="region"
        labels={labels}
        onHandlePointerDown={vi.fn()}
      />,
    );
    fireEvent.click(screen.getByTestId(CAPTURE_TOGGLE));
    expect(screen.getByTestId(CAPTURE_IMAGE)).toBeTruthy();
    fireEvent.pointerDown(screen.getAllByTestId("region")[0]);
    expect(screen.queryByTestId(CAPTURE_IMAGE)).toBeNull();
  });

  it("renders no alignment controls without labels", () => {
    render(
      <RegionOverlay
        items={captured}
        selectedIndex={null}
        draft={null}
        driftedIndexes={new Set([0])}
      />,
    );
    expect(screen.queryByTestId(CAPTURE_TOGGLE)).toBeNull();
    expect(screen.queryByTestId("region-drift-warning")).toBeNull();
  });
});

describe("RegionOverlay resizing and clipping", () => {
  const captured = [
    { index: 0, rect: { x: 1, y: 2, width: 30, height: 20 }, imageUrl: "blob:one" },
  ];

  it("hides the capture when a corner resize starts", () => {
    const onHandlePointerDown = vi.fn();
    render(
      <RegionOverlay
        items={captured}
        selectedIndex={null}
        draft={null}
        testId="region"
        labels={labels}
        onHandlePointerDown={onHandlePointerDown}
      />,
    );
    fireEvent.click(screen.getByTestId(CAPTURE_TOGGLE));
    expect(screen.getByTestId(CAPTURE_IMAGE)).toBeTruthy();
    fireEvent.pointerDown(screen.getAllByTestId("region-handle")[3]);
    expect(onHandlePointerDown).toHaveBeenCalledWith(0, "se", expect.anything());
    expect(screen.queryByTestId(CAPTURE_IMAGE)).toBeNull();
  });

  it("clips a region to the box the host gives, in the region's own coordinates", () => {
    const clipped = [
      {
        index: 0,
        rect: { x: 10, y: -20, width: 100, height: 50 },
        clip: { x: 0, y: 0, width: 300, height: 100 },
      },
    ];
    render(<RegionOverlay items={clipped} selectedIndex={null} draft={null} testId="region" />);
    expect(screen.getByTestId("region").style.clipPath).toBe(
      "polygon(-10px 20px, 290px 20px, 290px 120px, -10px 120px)",
    );
  });
});
