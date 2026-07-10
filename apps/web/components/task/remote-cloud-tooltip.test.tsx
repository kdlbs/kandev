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

describe("RemoteCloudTooltip relative timestamps", () => {
  it("renders created and last-check times as relative values", () => {
    const twoHoursAgo = new Date(Date.now() - 2 * 60 * 60 * 1000).toISOString();
    const fiveMinutesAgo = new Date(Date.now() - 5 * 60 * 1000).toISOString();

    render(
      <RemoteCloudTooltip
        taskId="task-1"
        executorType="sprites"
        fallbackName="Sprites.dev"
        status={{
          remote_name: "kandev-da7d150f-585",
          remote_state: "running",
          remote_created_at: twoHoursAgo,
          remote_checked_at: fiveMinutesAgo,
        }}
      />,
    );

    expect(screen.getByText("Created: 2h ago")).toBeTruthy();
    expect(screen.getByText("Last check: 5m ago")).toBeTruthy();
  });
});
