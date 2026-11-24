import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import type { ReactNode } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";

import type { AgentUpdateJob, AgentUpdatePreview, AgentUpdateStatus } from "@/lib/api";
import { AgentRuntimeUpdateControl } from "./agent-runtime-update-control";
import { RuntimeVersionSummary } from "./runtime-version-summary";

vi.mock("@/hooks/use-responsive-breakpoint", () => ({
  useResponsiveBreakpoint: () => ({ isMobile: false }),
}));

vi.mock("@/hooks/use-compact-task-chrome", () => ({ useTouchDrawer: () => false }));

vi.mock("@kandev/ui/tooltip", () => ({
  TooltipProvider: ({ children }: { children?: ReactNode }) => <>{children}</>,
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

const AGENT = "omp-acp";
const OMP_PACKAGE = "@oh-my-pi/pi-coding-agent";
const CANARY_LATEST = "18.22.0-canary.3";
const NATIVE_LINE = `agent-update-native-check-${AGENT}`;
const CHANNEL_LINE = `agent-update-channel-${AGENT}`;

function ompStatus(): AgentUpdateStatus {
  return {
    display_name: "omp",
    agent_name: AGENT,
    runtime_id: AGENT,
    package: OMP_PACKAGE,
    owner: "external",
    mechanism: "native",
    management: "manual",
    source: OMP_PACKAGE,
    current_version: "18.13.3",
    available: true,
    enabled: true,
    auto_update_supported: false,
    auto_update: false,
    update_mode: "self_update",
    default_version: "",
    effective_version: "18.13.3",
    check_state: "update_available",
    latest_version: CANARY_LATEST,
    check_source: "native",
  };
}

function nativePreview(overrides: Partial<AgentUpdatePreview> = {}): AgentUpdatePreview {
  return {
    update_mode: "self_update",
    agent_name: AGENT,
    package: OMP_PACKAGE,
    current_version: "18.13.3",
    effective_version: "18.13.3",
    target_version: "",
    operation: "update",
    available_versions: [],
    command: ["omp", "update"],
    command_string: "omp update",
    check_source: "native",
    check_state: "update_available",
    latest_version: CANARY_LATEST,
    update_channel: "canary",
    ...overrides,
  };
}

async function openNativeDialog(nextPreview: AgentUpdatePreview, onUpdate = vi.fn()) {
  render(
    <AgentRuntimeUpdateControl
      agentName={AGENT}
      displayName="omp"
      runtimeUpdate={{ supported: true, update_mode: "self_update", package: OMP_PACKAGE }}
      runtimeUpdateStatus={ompStatus()}
      onPreview={vi.fn().mockResolvedValue(nextPreview)}
      onUpdate={onUpdate}
    />,
  );
  fireEvent.click(screen.getByTestId(`agent-update-trigger-${AGENT}`));
  const confirm = await screen.findByTestId(`agent-update-confirm-${AGENT}`);
  await waitFor(() => expect((confirm as HTMLButtonElement).disabled).toBe(false));
  return confirm;
}

function unknownPreview(reason: AgentUpdatePreview["check_reason"]): AgentUpdatePreview {
  return nativePreview({ check_state: "unknown", latest_version: undefined, check_reason: reason });
}

afterEach(() => {
  cleanup();
  vi.clearAllMocks();
});

// @covers AC-AGENTS-RUNTIME-NATIVE-CHECK-002.4
describe("AgentRuntimeUpdateControl native update check", () => {
  it("shows the native latest and channel instead of the stable reference and keeps approval", async () => {
    const onUpdate = vi.fn().mockResolvedValue({
      job_id: "omp-job",
      agent_name: AGENT,
      update_mode: "self_update",
      status: "queued",
      started_at: "2026-11-17T12:00:00Z",
    } satisfies AgentUpdateJob);
    const confirm = await openNativeDialog(nativePreview(), onUpdate);
    expect(screen.getByTestId(NATIVE_LINE).textContent).toContain(CANARY_LATEST);
    expect(screen.getByTestId(CHANNEL_LINE).textContent).toContain("canary");
    expect(screen.queryByTestId(`agent-update-stable-reference-${AGENT}`)).toBeNull();
    expect(screen.queryByText(/stable latest reference/i)).toBeNull();
    fireEvent.click(confirm);
    await waitFor(() => expect(onUpdate).toHaveBeenCalledWith(AGENT, "", false, "self_update"));
  });

  it("shows up to date without a channel when none is reported", async () => {
    await openNativeDialog(
      nativePreview({
        check_state: "up_to_date",
        latest_version: undefined,
        update_channel: undefined,
      }),
    );
    expect(screen.getByTestId(NATIVE_LINE).textContent).toBe("Up to date.");
    expect(screen.queryByTestId(CHANNEL_LINE)).toBeNull();
  });

  it("explains an unknown result with its reason instead of the generic fallback", async () => {
    await openNativeDialog(unknownPreview("check_failed"));
    const reasonText = screen.getByTestId(NATIVE_LINE).textContent;
    cleanup();
    await openNativeDialog(unknownPreview(undefined));
    const fallbackText = screen.getByTestId(NATIVE_LINE).textContent;
    expect(reasonText).toBeTruthy();
    expect(reasonText).not.toBe(fallbackText);
  });
});

// The native result describes the installation before the job ran, so a
// finished job must not pair it with the newly installed version.
// @covers AC-AGENTS-RUNTIME-NATIVE-CHECK-002.4
describe("RuntimeVersionSummary after a finished native update job", () => {
  it.each(["succeeded", "failed"] as const)(
    "hides the pre-update native result once the job %s",
    (status) => {
      const job: AgentUpdateJob = {
        job_id: "omp-job",
        agent_name: AGENT,
        update_mode: "self_update",
        status,
        current_version: "18.23.0",
        started_at: "2026-11-17T12:00:00Z",
      };
      render(
        <RuntimeVersionSummary
          agentName={AGENT}
          preview={nativePreview()}
          job={job}
          changeVersion={null}
        />,
      );
      expect(screen.queryByTestId(NATIVE_LINE)).toBeNull();
      expect(screen.queryByTestId(CHANNEL_LINE)).toBeNull();
      if (status === "succeeded") {
        expect(screen.getByTestId(`agent-update-current-${AGENT}`).textContent).toContain(
          "18.23.0",
        );
      }
    },
  );

  it("keeps the native result while the job is still running", () => {
    const job: AgentUpdateJob = {
      job_id: "omp-job",
      agent_name: AGENT,
      update_mode: "self_update",
      status: "updating",
      started_at: "2026-11-17T12:00:00Z",
    };
    render(
      <RuntimeVersionSummary
        agentName={AGENT}
        preview={nativePreview()}
        job={job}
        changeVersion={null}
      />,
    );
    expect(screen.getByTestId(NATIVE_LINE).textContent).toContain(CANARY_LATEST);
  });
});
