import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import type { ReactNode } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";

import type { AgentUpdateJob, AgentUpdatePreview } from "@/lib/api";
import { AgentRuntimeUpdateControl } from "./agent-runtime-update-control";

vi.mock("@/hooks/use-responsive-breakpoint", () => ({
  useResponsiveBreakpoint: () => ({ isMobile: false }),
}));

vi.mock("@kandev/ui/tooltip", () => ({
  Tooltip: ({ children }: { children?: ReactNode }) => <>{children}</>,
  TooltipTrigger: ({ children }: { children?: ReactNode }) => <>{children}</>,
  TooltipContent: ({ children }: { children?: ReactNode }) => <>{children}</>,
}));

vi.mock("@kandev/ui/dialog", () => ({
  Dialog: ({ children, open }: { children?: ReactNode; open?: boolean }) =>
    open ? <>{children}</> : null,
  DialogContent: ({ children, ...props }: { children?: ReactNode } & Record<string, unknown>) => (
    <div {...props}>{children}</div>
  ),
  DialogDescription: ({ children }: { children?: ReactNode }) => <p>{children}</p>,
  DialogFooter: ({ children }: { children?: ReactNode }) => <footer>{children}</footer>,
  DialogHeader: ({ children }: { children?: ReactNode }) => <header>{children}</header>,
  DialogTitle: ({ children }: { children?: ReactNode }) => <h2>{children}</h2>,
}));

vi.mock("@kandev/ui/drawer", () => ({
  Drawer: ({ children, open }: { children?: ReactNode; open?: boolean }) =>
    open ? <>{children}</> : null,
  DrawerContent: ({ children }: { children?: ReactNode }) => <div>{children}</div>,
  DrawerDescription: ({ children }: { children?: ReactNode }) => <p>{children}</p>,
  DrawerFooter: ({ children }: { children?: ReactNode }) => <footer>{children}</footer>,
  DrawerHeader: ({ children }: { children?: ReactNode }) => <header>{children}</header>,
  DrawerTitle: ({ children }: { children?: ReactNode }) => <h2>{children}</h2>,
}));

const AGENT_NAME = "claude-acp";
const PACKAGE_NAME = "@agentclientprotocol/claude-agent-acp";
const ACTIVE_VERSION = "0.70.0";

function preview(overrides: Partial<AgentUpdatePreview> = {}): AgentUpdatePreview {
  return {
    agent_name: AGENT_NAME,
    package: PACKAGE_NAME,
    current_version: ACTIVE_VERSION,
    default_version: ACTIVE_VERSION,
    effective_version: ACTIVE_VERSION,
    target_version: "0.71.0",
    operation: "update",
    active_version: ACTIVE_VERSION,
    available_versions: [
      { version: "0.71.0", latest: true },
      { version: ACTIVE_VERSION, latest: false },
    ],
    command: ["npm", "exec"],
    command_string: "npm exec",
    ...overrides,
  };
}

function queuedJob(): AgentUpdateJob {
  return {
    job_id: "job-1",
    agent_name: AGENT_NAME,
    status: "queued",
    started_at: "2026-01-01T00:00:00.000Z",
  };
}

afterEach(() => {
  cleanup();
  vi.clearAllMocks();
});

describe("AgentRuntimeUpdateControl", () => {
  it("shows the update dot and exposes effective and latest versions", () => {
    render(
      <AgentRuntimeUpdateControl
        agentName={AGENT_NAME}
        displayName="Claude Code"
        runtimeUpdate={{
          supported: true,
          package: PACKAGE_NAME,
          default_version: ACTIVE_VERSION,
          effective_version: ACTIVE_VERSION,
        }}
        runtimeUpdateStatus={{
          agent_name: AGENT_NAME,
          package: PACKAGE_NAME,
          default_version: ACTIVE_VERSION,
          effective_version: ACTIVE_VERSION,
          latest_version: "0.71.0",
          check_state: "update_available",
        }}
        onPreview={vi.fn().mockResolvedValue(preview())}
        onUpdate={vi.fn().mockResolvedValue(queuedJob())}
      />,
    );

    expect(screen.getByTestId(`agent-update-available-dot-${AGENT_NAME}`)).toBeTruthy();
    expect(
      screen.getByRole("button", { name: `Update Claude Code from ${ACTIVE_VERSION} to 0.71.0` }),
    ).toBeTruthy();
  });

  it("keeps the trigger usable when status is unknown and offers the default action", async () => {
    const onPreview = vi.fn().mockResolvedValue(preview());
    const onUpdate = vi.fn().mockResolvedValue(queuedJob());
    render(
      <AgentRuntimeUpdateControl
        agentName={AGENT_NAME}
        displayName="Claude Code"
        runtimeUpdate={{
          supported: true,
          package: PACKAGE_NAME,
          default_version: ACTIVE_VERSION,
          effective_version: ACTIVE_VERSION,
        }}
        runtimeUpdateStatus={{
          agent_name: AGENT_NAME,
          package: PACKAGE_NAME,
          default_version: ACTIVE_VERSION,
          effective_version: ACTIVE_VERSION,
          check_state: "unknown",
        }}
        onPreview={onPreview}
        onUpdate={onUpdate}
      />,
    );

    expect(screen.queryByTestId(`agent-update-available-dot-${AGENT_NAME}`)).toBeNull();
    expect(
      screen.getByRole("button", {
        name: "Latest version for Claude Code is unavailable. The update control remains usable.",
      }),
    ).toBeTruthy();
    fireEvent.click(screen.getByTestId(`agent-update-trigger-${AGENT_NAME}`));
    await waitFor(() => expect(onPreview).toHaveBeenCalledWith(AGENT_NAME, undefined));
    expect(
      screen.getByRole("option", { name: `Use Kandev default (${ACTIVE_VERSION})` }),
    ).toBeTruthy();
  });

  it("offers the default action when the active version is unknown", async () => {
    const onPreview = vi.fn().mockResolvedValue(preview({ active_version: undefined }));
    render(
      <AgentRuntimeUpdateControl
        agentName={AGENT_NAME}
        displayName="Claude Code"
        runtimeUpdate={{
          supported: true,
          package: PACKAGE_NAME,
          default_version: ACTIVE_VERSION,
          effective_version: ACTIVE_VERSION,
        }}
        runtimeUpdateStatus={{
          agent_name: AGENT_NAME,
          package: PACKAGE_NAME,
          default_version: ACTIVE_VERSION,
          effective_version: ACTIVE_VERSION,
          check_state: "unknown",
        }}
        onPreview={onPreview}
        onUpdate={vi.fn().mockResolvedValue(queuedJob())}
      />,
    );

    fireEvent.click(screen.getByTestId(`agent-update-trigger-${AGENT_NAME}`));
    await waitFor(() => expect(onPreview).toHaveBeenCalledWith(AGENT_NAME, undefined));
    expect(
      screen.getByRole("option", { name: `Use Kandev default (${ACTIVE_VERSION})` }),
    ).toBeTruthy();
  });
});
