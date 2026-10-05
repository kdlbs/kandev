import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { useState, type ReactNode } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";

import type { AgentUpdateJob, AgentUpdatePreview, AgentUpdateStatus } from "@/lib/api";
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
const OMP_PACKAGE = "@oh-my-pi/pi-coding-agent";
const RUNTIME_STATUS_META = {
  display_name: "Claude",
  runtime_id: "npm:" + PACKAGE_NAME,
  owner: "kandev",
  mechanism: "npm_candidate",
  management: "managed",
  source: PACKAGE_NAME,
  guidance_url: "",
  current_version: ACTIVE_VERSION,
  available: true,
  enabled: true,
  auto_update_supported: true,
  auto_update: false,
} as const;

function preview(overrides: Partial<AgentUpdatePreview> = {}): AgentUpdatePreview {
  return {
    update_mode: "pinned",
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

function ompStatus(overrides: Partial<AgentUpdateStatus> = {}): AgentUpdateStatus {
  return {
    ...RUNTIME_STATUS_META,
    display_name: "omp",
    runtime_id: "omp-acp",
    current_version: "1.0.0",
    auto_update_supported: false,
    update_mode: "self_update",
    agent_name: "omp-acp",
    package: OMP_PACKAGE,
    default_version: "",
    effective_version: "",
    check_state: "unknown",
    ...overrides,
  };
}

function queuedJob(): AgentUpdateJob {
  return {
    update_mode: "pinned",
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
  it("keeps a long version history behind the browse action", async () => {
    const onPreview = vi.fn().mockResolvedValue(
      preview({
        available_versions: [
          { version: "0.71.0", latest: true },
          { version: ACTIVE_VERSION, latest: false },
          { version: "0.69.0", latest: false },
          { version: "0.50.0", latest: false },
        ],
      }),
    );
    render(
      <AgentRuntimeUpdateControl
        agentName={AGENT_NAME}
        displayName="Claude Code"
        runtimeUpdate={{
          supported: true,
          update_mode: "pinned",
          package: PACKAGE_NAME,
          default_version: ACTIVE_VERSION,
          effective_version: ACTIVE_VERSION,
        }}
        onPreview={onPreview}
        onUpdate={vi.fn().mockResolvedValue(queuedJob())}
      />,
    );

    fireEvent.click(screen.getByTestId(`agent-update-trigger-${AGENT_NAME}`));
    await waitFor(() => {
      expect(screen.getByRole("button", { name: "Browse all versions" })).toBeTruthy();
    });
    expect(screen.queryByText("0.50.0")).toBeNull();
  });

  it("filters the full version history and previews the selected version", async () => {
    const onPreview = vi.fn().mockResolvedValue(
      preview({
        available_versions: [
          { version: "0.71.0", latest: true },
          { version: ACTIVE_VERSION, latest: false },
          { version: "0.69.0", latest: false },
          { version: "0.50.0", latest: false },
        ],
      }),
    );
    render(
      <AgentRuntimeUpdateControl
        agentName={AGENT_NAME}
        displayName="Claude Code"
        runtimeUpdate={{
          supported: true,
          update_mode: "pinned",
          package: PACKAGE_NAME,
          default_version: ACTIVE_VERSION,
          effective_version: ACTIVE_VERSION,
        }}
        onPreview={onPreview}
        onUpdate={vi.fn().mockResolvedValue(queuedJob())}
      />,
    );

    fireEvent.click(screen.getByTestId(`agent-update-trigger-${AGENT_NAME}`));
    await waitFor(() => {
      expect(screen.getByRole("button", { name: "Browse all versions" })).toBeTruthy();
    });
    fireEvent.click(screen.getByRole("button", { name: "Browse all versions" }));
    const search = screen.getByPlaceholderText("Search versions");
    expect(screen.getByTestId(`agent-update-version-option-${AGENT_NAME}-0.50.0`)).toBeTruthy();
    fireEvent.change(search, { target: { value: "0.50" } });
    expect(screen.getByTestId(`agent-update-version-option-${AGENT_NAME}-0.50.0`)).toBeTruthy();
    expect(screen.queryByTestId(`agent-update-version-option-${AGENT_NAME}-0.69.0`)).toBeNull();

    fireEvent.click(screen.getByTestId(`agent-update-version-option-${AGENT_NAME}-0.50.0`));
    await waitFor(() => expect(onPreview).toHaveBeenLastCalledWith(AGENT_NAME, "0.50.0"));
  });
});

describe("AgentRuntimeUpdateControl", () => {
  it("shows the update dot and exposes effective and latest versions", () => {
    render(
      <AgentRuntimeUpdateControl
        agentName={AGENT_NAME}
        displayName="Claude Code"
        runtimeUpdate={{
          supported: true,
          update_mode: "pinned",
          package: PACKAGE_NAME,
          default_version: ACTIVE_VERSION,
          effective_version: ACTIVE_VERSION,
        }}
        runtimeUpdateStatus={{
          ...RUNTIME_STATUS_META,
          agent_name: AGENT_NAME,
          update_mode: "pinned",
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
          update_mode: "pinned",
          package: PACKAGE_NAME,
          default_version: ACTIVE_VERSION,
          effective_version: ACTIVE_VERSION,
        }}
        runtimeUpdateStatus={{
          ...RUNTIME_STATUS_META,
          agent_name: AGENT_NAME,
          update_mode: "pinned",
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
      screen.getByRole("button", { name: `Use Kandev default (${ACTIVE_VERSION})` }),
    ).toBeTruthy();
  });
});

describe("AgentRuntimeUpdateControl unknown active version", () => {
  it("offers the default action when the active version is unknown", async () => {
    const onPreview = vi.fn().mockResolvedValue(preview({ active_version: undefined }));
    render(
      <AgentRuntimeUpdateControl
        agentName={AGENT_NAME}
        displayName="Claude Code"
        runtimeUpdate={{
          supported: true,
          update_mode: "pinned",
          package: PACKAGE_NAME,
          default_version: ACTIVE_VERSION,
          effective_version: ACTIVE_VERSION,
        }}
        runtimeUpdateStatus={{
          ...RUNTIME_STATUS_META,
          agent_name: AGENT_NAME,
          update_mode: "pinned",
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
      screen.getByRole("button", { name: `Use Kandev default (${ACTIVE_VERSION})` }),
    ).toBeTruthy();
  });
});

describe("AgentRuntimeUpdateControl reset state", () => {
  it("shows the cleared active version and job effective version after a reset", async () => {
    const defaultVersion = "0.71.0";
    const onPreview = vi.fn().mockResolvedValue(
      preview({
        default_version: defaultVersion,
        target_version: defaultVersion,
        operation: "use_default",
      }),
    );
    const resetJob: AgentUpdateJob = {
      job_id: "job-reset",
      update_mode: "pinned",
      agent_name: AGENT_NAME,
      status: "succeeded",
      operation: "use_default",
      current_version: defaultVersion,
      default_version: defaultVersion,
      effective_version: defaultVersion,
      target_version: defaultVersion,
      started_at: "2026-01-01T00:00:00.000Z",
      finished_at: "2026-01-01T00:01:00.000Z",
    };
    function StatefulControl() {
      const [job, setJob] = useState<AgentUpdateJob>();
      const update = vi.fn().mockImplementation(async () => {
        setJob(resetJob);
        return resetJob;
      });
      return (
        <AgentRuntimeUpdateControl
          agentName={AGENT_NAME}
          displayName="Claude Code"
          runtimeUpdate={{
            supported: true,
            update_mode: "pinned",
            package: PACKAGE_NAME,
            default_version: defaultVersion,
            active_version: ACTIVE_VERSION,
            effective_version: ACTIVE_VERSION,
          }}
          onPreview={onPreview}
          onUpdate={update}
          job={job}
        />
      );
    }

    render(<StatefulControl />);
    fireEvent.click(screen.getByTestId(`agent-update-trigger-${AGENT_NAME}`));
    await waitFor(() =>
      expect(
        screen.getByRole("button", { name: `Use Kandev default (${defaultVersion})` }),
      ).toBeTruthy(),
    );
    fireEvent.click(screen.getByTestId(`agent-update-quick-default-${AGENT_NAME}`));
    await waitFor(() => expect(onPreview).toHaveBeenLastCalledWith(AGENT_NAME, undefined, true));
    fireEvent.click(screen.getByTestId(`agent-update-confirm-${AGENT_NAME}`));

    await waitFor(() => {
      const dialog = screen.getByTestId(`agent-update-dialog-${AGENT_NAME}`);
      const text = dialog.textContent ?? "";
      expect(text).toContain(`Effective version: ${defaultVersion}`);
      expect(text).not.toContain(`Active version: ${ACTIVE_VERSION}`);
    });
  });
});

describe("AgentRuntimeUpdateControl self-update", () => {
  it.each(["update", "repair"] as const)(
    "shows reference and approves targetless %s from the control",
    async (operation) => {
      const agentName = "omp-acp";
      const onUpdate = vi.fn().mockResolvedValue({
        job_id: "omp-job",
        agent_name: agentName,
        update_mode: "self_update",
        status: "queued",
        started_at: "2026-09-26T12:00:00Z",
      } satisfies AgentUpdateJob);
      render(
        <AgentRuntimeUpdateControl
          agentName={agentName}
          displayName="omp"
          runtimeUpdate={{
            supported: true,
            update_mode: "self_update",
            package: OMP_PACKAGE,
          }}
          runtimeUpdateStatus={ompStatus({ agent_name: agentName })}
          onPreview={vi.fn().mockResolvedValue({
            update_mode: "self_update",
            agent_name: agentName,
            package: OMP_PACKAGE,
            current_version: operation === "repair" ? "" : "1.0.0",
            target_version: "",
            stable_latest_version: "1.1.0",
            operation,
            available_versions: [],
            command: ["omp", "update"],
            command_string: "omp update",
          } satisfies AgentUpdatePreview)}
          onUpdate={onUpdate}
        />,
      );
      fireEvent.click(screen.getByTestId(`agent-update-trigger-${agentName}`));
      const confirm = await screen.findByTestId(`agent-update-confirm-${agentName}`);
      await waitFor(() => expect((confirm as HTMLButtonElement).disabled).toBe(false));
      expect(screen.queryByTestId(`agent-update-version-picker-${agentName}`)).toBeNull();
      expect(
        screen.getByTestId(`agent-update-stable-reference-${agentName}`).textContent,
      ).toContain("1.1.0");
      expect(screen.getByText(/configured channel.*different version/i)).toBeTruthy();
      fireEvent.click(confirm);
      await waitFor(() =>
        expect(onUpdate).toHaveBeenCalledWith(agentName, "", false, "self_update"),
      );
    },
  );
});

describe("AgentRuntimeUpdateControl self-update trigger", () => {
  it("does not describe the stable reference as the guaranteed update target", () => {
    render(
      <AgentRuntimeUpdateControl
        agentName="omp-acp"
        displayName="omp"
        runtimeUpdate={{
          supported: true,
          update_mode: "self_update",
          package: OMP_PACKAGE,
        }}
        runtimeUpdateStatus={ompStatus({
          latest_version: "1.1.0",
          check_state: "update_available",
        })}
        onPreview={vi.fn()}
        onUpdate={vi.fn()}
      />,
    );
    expect(screen.getByTestId("agent-update-trigger-omp-acp").getAttribute("aria-label")).toBe(
      "Update omp",
    );
  });
});

describe("AgentRuntimeUpdateControl self-update results", () => {
  it("keeps a metadata-unknown trigger usable and shows a preview error without inventing a job", async () => {
    const onPreview = vi.fn().mockRejectedValue(new Error("Registry unavailable"));
    const onUpdate = vi.fn();
    render(
      <AgentRuntimeUpdateControl
        agentName="omp-acp"
        displayName="omp"
        runtimeUpdate={{
          supported: true,
          update_mode: "self_update",
          package: OMP_PACKAGE,
        }}
        runtimeUpdateStatus={ompStatus()}
        onPreview={onPreview}
        onUpdate={onUpdate}
      />,
    );
    const trigger = screen.getByTestId("agent-update-trigger-omp-acp");
    expect((trigger as HTMLButtonElement).disabled).toBe(false);
    fireEvent.click(trigger);
    expect((await screen.findByRole("alert")).textContent).toContain("Registry unavailable");
    expect(screen.queryByTestId("agent-update-result-omp-acp")).toBeNull();
    expect(onUpdate).not.toHaveBeenCalled();
  });

  it("shows an empty-ID up-to-date result locally even when status refresh is pending elsewhere", async () => {
    const terminal: AgentUpdateJob = {
      update_mode: "self_update",
      agent_name: "omp-acp",
      job_id: "",
      operation: "up_to_date",
      status: "succeeded",
      started_at: "2026-09-26T12:00:00Z",
    };
    render(
      <AgentRuntimeUpdateControl
        agentName="omp-acp"
        displayName="omp"
        runtimeUpdate={{
          supported: true,
          update_mode: "self_update",
          package: OMP_PACKAGE,
        }}
        onPreview={vi.fn().mockResolvedValue({
          agent_name: "omp-acp",
          update_mode: "self_update",
          package: OMP_PACKAGE,
          current_version: "1.1.0",
          target_version: "",
          stable_latest_version: "1.1.0",
          operation: "repair",
          available_versions: [],
          command: ["omp", "update"],
          command_string: "omp update",
        } satisfies AgentUpdatePreview)}
        onUpdate={vi.fn().mockResolvedValue(terminal)}
      />,
    );
    fireEvent.click(screen.getByTestId("agent-update-trigger-omp-acp"));
    const confirm = await screen.findByTestId("agent-update-confirm-omp-acp");
    await waitFor(() => expect((confirm as HTMLButtonElement).disabled).toBe(false));
    fireEvent.click(confirm);
    expect((await screen.findByTestId("agent-update-result-omp-acp")).textContent).toContain(
      "already up to date",
    );
    expect(screen.queryByTestId("agent-update-confirm-omp-acp")).toBeNull();
  });
});
