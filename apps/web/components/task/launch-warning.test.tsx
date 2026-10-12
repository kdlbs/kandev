import { afterEach, describe, expect, it, vi } from "vitest";
import { cleanup, render, screen } from "@testing-library/react";
import { StateProvider } from "@/components/state-provider";
import type { LaunchWarningEntry } from "@/lib/state/slices/session-runtime/types";
import { LaunchWarning } from "./launch-warning";

const SESSION_ID = "session-1";

function makeEntry(overrides: Partial<LaunchWarningEntry> = {}): LaunchWarningEntry {
  return {
    executorId: "executor-1",
    host: "10.0.0.5",
    state: "unreachable",
    reason: "timeout",
    ...overrides,
  };
}

afterEach(() => {
  cleanup();
  vi.useRealTimers();
});

function renderWithEntry(entry?: LaunchWarningEntry) {
  return render(
    <StateProvider
      initialState={{
        launchWarning: { bySessionId: entry ? { [SESSION_ID]: entry } : {} },
      }}
    >
      <LaunchWarning sessionId={SESSION_ID} />
    </StateProvider>,
  );
}

describe("LaunchWarning", () => {
  it("renders nothing when no launch warning was published for this session", () => {
    const { container } = renderWithEntry(undefined);
    expect(container.textContent).toBe("");
  });

  it("names the probed host from the event payload", () => {
    renderWithEntry(makeEntry());
    expect(screen.getByTestId("launch-warning").textContent).toContain("10.0.0.5");
  });

  it("shows the age of the last successful probe", () => {
    vi.useFakeTimers();
    vi.setSystemTime(new Date("2026-06-11T00:05:00.000Z"));
    try {
      renderWithEntry(makeEntry({ lastSuccessAt: "2026-06-11T00:00:00.000Z" }));
      expect(screen.getByTestId("launch-warning-last-success").textContent).toMatch(/5m/);
    } finally {
      vi.useRealTimers();
    }
  });

  it("states plainly that no successful probe was ever recorded, not a zero age", () => {
    renderWithEntry(makeEntry({ lastSuccessAt: undefined }));
    const text = screen.getByTestId("launch-warning-last-success").textContent ?? "";
    expect(text).not.toMatch(/0[ms]/);
    expect(text.length).toBeGreaterThan(0);
  });

  it.each(["config", "timeout", "host_key", "auth", "network", "unknown"])(
    "renders a reason sentence for %s without throwing",
    (reason) => {
      renderWithEntry(makeEntry({ reason }));
      expect(screen.getByTestId("launch-warning-reason").textContent?.length).toBeGreaterThan(0);
    },
  );

  it("never renders any confirmation control — the launch is never blocked", () => {
    renderWithEntry(makeEntry());
    expect(screen.queryByRole("button")).toBeNull();
  });
});
