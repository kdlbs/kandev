import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { RemoteCloudTooltip } from "./remote-cloud-tooltip";

afterEach(() => cleanup());

vi.mock("@kandev/ui/tooltip", () => ({
  Tooltip: ({ children }: { children: React.ReactNode }) => <>{children}</>,
  TooltipTrigger: ({ children }: { children: React.ReactNode }) => <>{children}</>,
  TooltipContent: ({ children }: { children: React.ReactNode }) => <>{children}</>,
}));

describe("RemoteCloudTooltip executor icon", () => {
  it("shows a container icon for Docker executors", () => {
    render(
      <RemoteCloudTooltip
        taskId="task-1"
        executorType="local_docker"
        fallbackName="Docker"
        status={{ remote_checked_at: new Date().toISOString() }}
      />,
    );

    expect(screen.getByTestId("executor-status-container-icon")).toBeTruthy();
    expect(screen.queryByTestId("executor-status-cloud-icon")).toBeNull();
  });

  it("keeps the cloud icon for Sprites executors", () => {
    render(
      <RemoteCloudTooltip
        taskId="task-1"
        executorType="sprites"
        fallbackName="Sprites.dev"
        status={{ remote_checked_at: new Date().toISOString() }}
      />,
    );

    expect(screen.getByTestId("executor-status-cloud-icon")).toBeTruthy();
  });
});
