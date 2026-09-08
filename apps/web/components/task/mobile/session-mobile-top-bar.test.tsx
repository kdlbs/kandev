import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";

vi.mock("@/components/task/executor-settings-button", () => ({
  ExecutorSettingsButton: ({ taskId, sessionId }: { taskId: string; sessionId: string }) => (
    <button data-testid="mobile-kubernetes-executor">
      {taskId}:{sessionId}
    </button>
  ),
}));

vi.mock("@/components/task/remote-cloud-tooltip", () => ({
  RemoteCloudTooltip: () => <span data-testid="mobile-remote-tooltip" />,
}));

import * as mobileTopBar from "./session-mobile-top-bar";

afterEach(cleanup);

describe("SessionMobileTopBar remote executor disclosure", () => {
  it("uses the touch-capable Kubernetes disclosure instead of a tooltip-only glyph", () => {
    expect("MobileRemoteExecutorIndicator" in mobileTopBar).toBe(true);
    const MobileRemoteExecutorIndicator = (
      mobileTopBar as typeof mobileTopBar & {
        MobileRemoteExecutorIndicator: React.ComponentType<{
          taskId: string;
          sessionId: string;
          remoteExecutorType: string;
        }>;
      }
    ).MobileRemoteExecutorIndicator;

    const { rerender } = render(
      <MobileRemoteExecutorIndicator
        taskId="task-1"
        sessionId="session-1"
        remoteExecutorType="k8s"
      />,
    );
    expect(screen.getByTestId("mobile-kubernetes-executor").textContent).toBe("task-1:session-1");
    expect(screen.queryByTestId("mobile-remote-tooltip")).toBeNull();

    rerender(
      <MobileRemoteExecutorIndicator
        taskId="task-1"
        sessionId="session-1"
        remoteExecutorType="ssh"
      />,
    );
    expect(screen.getByTestId("mobile-remote-tooltip")).toBeTruthy();
    expect(screen.queryByTestId("mobile-kubernetes-executor")).toBeNull();
  });
});
