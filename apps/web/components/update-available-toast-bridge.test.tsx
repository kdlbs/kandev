import { act, cleanup, render } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { ComponentProps } from "react";
import type { UpdateAvailableNotification } from "@/lib/state/slices/ui/types";

const mocks = vi.hoisted(() => ({
  toast: vi.fn(),
  refresh: vi.fn(),
  statuses: vi.fn(),
  pathname: "/",
  state: {
    updateJobs: { byAgent: {} },
    updateAvailableNotification: null as UpdateAvailableNotification | null,
  },
}));

vi.mock("@/components/state-provider", () => ({
  useAppStore: (selector: (state: typeof mocks.state) => unknown) => selector(mocks.state),
}));
vi.mock("@/hooks/use-update-available-toast", () => ({
  useUpdateAvailableToast: mocks.toast,
}));
vi.mock("@/hooks/domains/settings/use-agent-runtime-update-statuses", () => ({
  useAgentRuntimeUpdateStatuses: mocks.statuses,
}));
vi.mock("@/lib/routing/client-router", () => ({ usePathname: () => mocks.pathname }));
vi.mock("@/components/routing/app-link", () => ({
  default: (props: ComponentProps<"a">) => <a {...props} />,
}));

import { UpdateAvailableToastBridge } from "./update-available-toast-bridge";

beforeEach(() => {
  vi.useFakeTimers();
  vi.clearAllMocks();
  mocks.pathname = "/";
  mocks.state.updateAvailableNotification = null;
  mocks.statuses.mockReturnValue({
    refresh: mocks.refresh,
    statusByAgent: {
      gemini: { available: true, enabled: true, check_state: "update_available" },
    },
  });
});

afterEach(() => {
  cleanup();
  vi.useRealTimers();
});

describe("UpdateAvailableToastBridge", () => {
  // @covers AC-AGENTS-RUNTIME-NOTIFY-001.7
  it.each(["/", "/tasks", "/settings/agents"])(
    "consumes runtime updates without floating controls on %s",
    (pathname) => {
      mocks.pathname = pathname;
      const { container } = render(<UpdateAvailableToastBridge />);
      expect(container.childElementCount).toBe(0);
      expect(mocks.toast).toHaveBeenCalled();
      expect(mocks.statuses).toHaveBeenCalledWith(mocks.state.updateJobs.byAgent);
    },
  );

  it("keeps discovery active when there are no available updates", () => {
    mocks.statuses.mockReturnValue({ refresh: mocks.refresh, statusByAgent: {} });
    const { container } = render(<UpdateAvailableToastBridge />);
    expect(container.childElementCount).toBe(0);
    act(() => vi.advanceTimersByTime(60_000));
    expect(mocks.refresh).toHaveBeenCalledTimes(1);
  });

  // @covers AC-AGENTS-RUNTIME-NOTIFY-001.2
  it("refreshes periodically and releases its timer on unmount", () => {
    const { unmount } = render(<UpdateAvailableToastBridge />);
    act(() => vi.advanceTimersByTime(120_000));
    expect(mocks.refresh).toHaveBeenCalledTimes(2);
    unmount();
    act(() => vi.advanceTimersByTime(60_000));
    expect(mocks.refresh).toHaveBeenCalledTimes(2);
    expect(vi.getTimerCount()).toBe(0);
  });

  it("refreshes on a runtime notice without rendering a control", () => {
    const { rerender, container } = render(<UpdateAvailableToastBridge />);
    mocks.state.updateAvailableNotification = {
      agent_name: "gemini",
      occurrence_id: "runtime-gemini-2",
      version: "2.0.0",
      title: "Gemini runtime update available",
      body: "A new version is available.",
    };
    rerender(<UpdateAvailableToastBridge />);
    expect(mocks.refresh).toHaveBeenCalledTimes(1);
    expect(container.childElementCount).toBe(0);
  });

  it("does not refresh runtime discovery for an application release notice", () => {
    mocks.state.updateAvailableNotification = {
      occurrence_id: "release-1",
      version: "1.0.0",
      title: "Kandev update available",
      body: "A new release is available.",
    };
    render(<UpdateAvailableToastBridge />);
    expect(mocks.toast).toHaveBeenCalled();
    expect(mocks.refresh).not.toHaveBeenCalled();
  });
});
