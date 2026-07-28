import { act, cleanup, fireEvent, render, waitFor } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { PullToRefresh } from "./pull-to-refresh";

function touch(clientX: number, clientY: number) {
  return [{ clientX, clientY }];
}

describe("PullToRefresh", () => {
  afterEach(cleanup);

  it("refreshes after an eligible downward pull", async () => {
    const onRefresh = vi.fn().mockResolvedValue(undefined);
    const { getByTestId } = render(<PullToRefresh onRefresh={onRefresh}>Tasks</PullToRefresh>);
    const root = getByTestId("pull-to-refresh");

    fireEvent.touchStart(root, { touches: touch(20, 0) });
    fireEvent.touchMove(root, { touches: touch(20, 140) });
    expect(getByTestId("pull-to-refresh-indicator")).toBeTruthy();
    fireEvent.touchEnd(root);

    await waitFor(() => expect(onRefresh).toHaveBeenCalledTimes(1));
  });

  it("does not refresh cancelled, horizontal, or below-threshold gestures", () => {
    const onRefresh = vi.fn();
    const { getByTestId } = render(<PullToRefresh onRefresh={onRefresh}>Tasks</PullToRefresh>);
    const root = getByTestId("pull-to-refresh");

    fireEvent.touchStart(root, { touches: touch(20, 0) });
    fireEvent.touchMove(root, { touches: touch(20, 40) });
    fireEvent.touchEnd(root);
    fireEvent.touchStart(root, { touches: touch(20, 0) });
    fireEvent.touchMove(root, { touches: touch(140, 20) });
    fireEvent.touchEnd(root);
    fireEvent.touchStart(root, { touches: touch(20, 0) });
    fireEvent.touchMove(root, { touches: touch(20, 140) });
    fireEvent.touchCancel(root);

    expect(onRefresh).not.toHaveBeenCalled();
  });

  it("ignores a second gesture while refreshing", async () => {
    let resolveRefresh!: () => void;
    const onRefresh = vi.fn(() => new Promise<void>((resolve) => (resolveRefresh = resolve)));
    const { getByTestId } = render(<PullToRefresh onRefresh={onRefresh}>Tasks</PullToRefresh>);
    const root = getByTestId("pull-to-refresh");
    const pull = () => {
      fireEvent.touchStart(root, { touches: touch(20, 0) });
      fireEvent.touchMove(root, { touches: touch(20, 140) });
      fireEvent.touchEnd(root);
    };

    pull();
    pull();
    expect(onRefresh).toHaveBeenCalledTimes(1);
    await act(async () => resolveRefresh());
  });
});
