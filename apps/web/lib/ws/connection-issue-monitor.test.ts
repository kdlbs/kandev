import { afterEach, describe, expect, it, vi } from "vitest";
import { ConnectionIssueMonitor } from "./connection-issue-monitor";

describe("ConnectionIssueMonitor", () => {
  afterEach(() => vi.useRealTimers());

  it("waits three seconds before reporting an unstable connection", () => {
    vi.useFakeTimers();
    const onChange = vi.fn();
    const monitor = new ConnectionIssueMonitor(onChange);

    monitor.onStatusChange("connecting");
    vi.advanceTimersByTime(2_999);

    expect(onChange).not.toHaveBeenCalled();

    vi.advanceTimersByTime(1);

    expect(onChange).toHaveBeenCalledWith("unstable");
  });

  it("keeps one outage clock across reconnect lifecycle transitions", () => {
    vi.useFakeTimers();
    const onChange = vi.fn();
    const monitor = new ConnectionIssueMonitor(onChange);

    monitor.onStatusChange("reconnecting");
    vi.advanceTimersByTime(3_000);
    monitor.onStatusChange("connecting");
    monitor.onStatusChange("error");
    vi.advanceTimersByTime(7_000);

    expect(onChange).toHaveBeenNthCalledWith(1, "unstable");
    expect(onChange).toHaveBeenNthCalledWith(2, "lost");
  });

  it("clears immediately on connection and starts a later outage fresh", () => {
    vi.useFakeTimers();
    const onChange = vi.fn();
    const monitor = new ConnectionIssueMonitor(onChange);

    monitor.onStatusChange("disconnected");
    vi.advanceTimersByTime(3_000);
    monitor.onStatusChange("connected");
    monitor.onStatusChange("reconnecting");
    vi.advanceTimersByTime(2_999);

    expect(onChange).toHaveBeenNthCalledWith(1, "unstable");
    expect(onChange).toHaveBeenNthCalledWith(2, "none");
    expect(onChange).toHaveBeenCalledTimes(2);

    vi.advanceTimersByTime(1);

    expect(onChange).toHaveBeenNthCalledWith(3, "unstable");
    expect(onChange).toHaveBeenCalledTimes(3);
  });

  it("clears scheduled timers and ignores later status changes after disposal", () => {
    vi.useFakeTimers();
    const onChange = vi.fn();
    const monitor = new ConnectionIssueMonitor(onChange);

    monitor.onStatusChange("reconnecting");
    monitor.dispose();
    monitor.onStatusChange("reconnecting");
    vi.advanceTimersByTime(10_000);

    expect(onChange).not.toHaveBeenCalled();
  });
});
