import { act, cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { useState, type ReactNode } from "react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import type { AgentUpdatePreview, AgentUpdateStatus } from "@/lib/api";
import { ApiError } from "@/lib/api/client";
import type { RuntimeUpdate } from "@/lib/types/http";
import { StateProvider } from "@/components/state-provider";
import type { RuntimeUpdateManagerView } from "@/hooks/domains/settings/use-agent-runtime-update-manager";
import { AgentRuntimeUpdateManager } from "./agent-runtime-update-manager";
import { RuntimePolicyControllers } from "./agent-runtime-policies";
import { SettingsSaveProvider } from "./settings-save-provider";

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
  DrawerContent: ({ children, ...props }: { children?: ReactNode } & Record<string, unknown>) => (
    <div {...props}>{children}</div>
  ),
  DrawerDescription: ({ children }: { children?: ReactNode }) => <p>{children}</p>,
  DrawerFooter: ({ children }: { children?: ReactNode }) => <footer>{children}</footer>,
  DrawerHeader: ({ children }: { children?: ReactNode }) => <header>{children}</header>,
  DrawerTitle: ({ children }: { children?: ReactNode }) => <h2>{children}</h2>,
}));
vi.mock("@/hooks/domains/auth/use-is-admin", () => ({ useIsAdmin: () => true }));

const updateAgentMock = vi.fn();
const listJobsMock = vi.fn();
vi.mock("@/lib/api", async (importOriginal) => {
  const actual = await importOriginal<typeof import("@/lib/api")>();
  return {
    ...actual,
    updateAgent: (...args: unknown[]) => updateAgentMock(...args),
    listAgentUpdateJobs: (...args: unknown[]) => listJobsMock(...args),
  };
});

const runtimeUpdate: RuntimeUpdate = {
  supported: true,
  update_mode: "pinned",
  package: "opencode-ai",
};

const AGENT_NAME = "claude-acp";
const MANAGER_DIALOG = "runtime-manager-dialog";
const CLAUDE_DETAIL = "agent-update-dialog-claude-acp";
const CLAUDE_MANAGE = "runtime-manage-versions-claude-acp";
const BULK_APPROVE = "runtime-bulk-approve";
const CLAUDE_PACKAGE = "@example/claude";
const CODEX_PACKAGE = "@example/codex";
const CLAUDE_JOB = "job-claude";
const STARTED_AT = "2026-01-01T00:00:00Z";

function status(overrides: Partial<AgentUpdateStatus> = {}): AgentUpdateStatus {
  return {
    agent_name: AGENT_NAME,
    display_name: "Claude",
    runtime_id: "npm:@example/claude",
    owner: "kandev",
    mechanism: "npm_candidate",
    management: "managed",
    update_mode: "pinned",
    package: CLAUDE_PACKAGE,
    source: CLAUDE_PACKAGE,
    available: true,
    enabled: true,
    auto_update_supported: true,
    auto_update: false,
    default_version: "1.0.0",
    effective_version: "1.0.0",
    current_version: "1.0.0",
    latest_version: "1.1.0",
    check_state: "update_available",
    ...overrides,
  };
}

function preview(agentName: string, target = "1.1.0", pkg = CLAUDE_PACKAGE): AgentUpdatePreview {
  return {
    update_mode: "pinned",
    agent_name: agentName,
    package: pkg,
    current_version: "1.0.0",
    default_version: "1.0.0",
    effective_version: "1.0.0",
    active_version: "1.0.0",
    target_version: target,
    operation: "update",
    reviewed_runtime: {
      runtime_id: `npm:${pkg}`,
      package: pkg,
      current_version: "1.0.0",
      effective_version: "1.0.0",
      active_version: "1.0.0",
    },
    available_versions: [
      { version: target, latest: true },
      { version: "1.0.0", latest: false },
    ],
    command: ["npm", "exec"],
    command_string: "npm exec",
  };
}

type PreviewFn = (
  name: string,
  target?: string,
  useDefault?: boolean,
  family?: "v2",
) => Promise<AgentUpdatePreview>;

function Harness({
  statuses,
  isMobile = false,
  onPreview,
  onStartUpdate = vi.fn(),
  refreshResult = true,
  initialView = { kind: "overview" },
}: {
  statuses: AgentUpdateStatus[];
  isMobile?: boolean;
  onPreview: PreviewFn;
  onStartUpdate?: (
    agentName: string,
    targetVersion: string,
    ...args: unknown[]
  ) => Promise<import("@/lib/api").AgentUpdateJob>;
  refreshResult?: boolean;
  initialView?: RuntimeUpdateManagerView;
}) {
  const [view, setView] = useState<RuntimeUpdateManagerView>(initialView);
  return (
    <StateProvider>
      <SettingsSaveProvider>
        <RuntimePolicyControllers statuses={statuses}>
          <AgentRuntimeUpdateManager
            view={view}
            isMobile={isMobile}
            statuses={statuses}
            statusLoading={false}
            refreshFailed={false}
            updateJobs={{}}
            installJobs={{}}
            canManage
            hasRuntimeControl={() => true}
            resolveDisplayName={(name) => (name === AGENT_NAME ? "Claude" : "Codex")}
            resolveRuntimeUpdate={(name) =>
              name === AGENT_NAME ? runtimeUpdate : { ...runtimeUpdate, package: CODEX_PACKAGE }
            }
            previewUpdate={onPreview}
            startUpdate={onStartUpdate}
            refresh={() => Promise.resolve(refreshResult)}
            onClose={() => setView({ kind: "closed" })}
            onBack={() => setView({ kind: "overview" })}
            onOpenRuntime={(agentName) => setView({ kind: "runtime", agentName })}
          />
        </RuntimePolicyControllers>
      </SettingsSaveProvider>
    </StateProvider>
  );
}

afterEach(cleanup);

beforeEach(() => {
  updateAgentMock.mockReset();
  listJobsMock.mockReset();
  listJobsMock.mockResolvedValue({ jobs: [] });
});

describe("AgentRuntimeUpdateManager", () => {
  it("opens directly on a runtime and returns to the overview without a second surface", async () => {
    const statuses = [status()];
    const { rerender } = render(
      <Harness statuses={statuses} onPreview={(name) => Promise.resolve(preview(name))} />,
    );
    expect(screen.getByTestId(MANAGER_DIALOG)).toBeTruthy();
    fireEvent.click(screen.getByTestId(CLAUDE_MANAGE));
    await waitFor(() => expect(screen.getByTestId(CLAUDE_DETAIL)).toBeTruthy());
    expect(screen.queryByTestId(MANAGER_DIALOG)).toBeNull();
    fireEvent.click(screen.getByTestId("agent-update-back-claude-acp"));
    await waitFor(() => expect(screen.getByTestId(MANAGER_DIALOG)).toBeTruthy());
    rerender(<Harness statuses={statuses} onPreview={(name) => Promise.resolve(preview(name))} />);
  });

  it("blocks update approval while the selected runtime policy is dirty", async () => {
    render(<Harness statuses={[status()]} onPreview={(name) => Promise.resolve(preview(name))} />);
    fireEvent.click(screen.getByTestId(CLAUDE_MANAGE));
    const confirm = await screen.findByTestId("agent-update-confirm-claude-acp");
    await waitFor(() => expect((confirm as HTMLButtonElement).disabled).toBe(false));
    fireEvent.click(screen.getByRole("switch"));
    await waitFor(() => expect(screen.getByTestId("runtime-policy-dirty-footer")).toBeTruthy());
    expect((confirm as HTMLButtonElement).disabled).toBe(true);
    expect(confirm.getAttribute("title")).toContain("Save or reset");
  });

  it("preserves the selected runtime when the viewport changes to phone", async () => {
    const { rerender } = render(
      <Harness statuses={[status()]} onPreview={(name) => Promise.resolve(preview(name))} />,
    );
    fireEvent.click(screen.getByTestId(CLAUDE_MANAGE));
    await waitFor(() => expect(screen.getByTestId(CLAUDE_DETAIL)).toBeTruthy());
    rerender(
      <Harness
        statuses={[status()]}
        isMobile
        onPreview={(name) => Promise.resolve(preview(name))}
      />,
    );
    expect(screen.getByTestId("agent-update-drawer-claude-acp")).toBeTruthy();
    expect(screen.queryByTestId(CLAUDE_DETAIL)).toBeNull();
  });

  it("does not attach a late preview from another runtime to the current detail", async () => {
    const claude = Promise.withResolvers<AgentUpdatePreview>();
    const codex = Promise.withResolvers<AgentUpdatePreview>();
    const statuses = [
      status(),
      status({
        agent_name: "codex-acp",
        display_name: "Codex",
        runtime_id: "npm:@example/codex",
        package: CODEX_PACKAGE,
        source: CODEX_PACKAGE,
      }),
    ];
    render(
      <Harness
        statuses={statuses}
        onPreview={(name) => (name === AGENT_NAME ? claude.promise : codex.promise)}
      />,
    );
    fireEvent.click(screen.getByTestId(CLAUDE_MANAGE));
    await waitFor(() => expect(screen.getByTestId(CLAUDE_DETAIL)).toBeTruthy());
    fireEvent.click(screen.getByTestId("agent-update-back-claude-acp"));
    fireEvent.click(screen.getByTestId("runtime-manage-versions-codex-acp"));
    await waitFor(() => expect(screen.getByTestId("agent-update-dialog-codex-acp")).toBeTruthy());

    await act(async () => {
      codex.resolve(preview("codex-acp", "2.1.0"));
    });
    await waitFor(() =>
      expect(screen.getByTestId("agent-update-dialog-codex-acp").textContent).toContain("2.1.0"),
    );
    await act(async () => {
      claude.resolve(preview(AGENT_NAME));
    });
    expect(screen.getByTestId("agent-update-dialog-codex-acp").textContent).toContain("2.1.0");
    expect(screen.queryByTestId(CLAUDE_DETAIL)).toBeNull();
  });
});

describe("AgentRuntimeUpdateManager bulk review", () => {
  it("reviews exact targets, preserves deselection, and submits only selected rows", async () => {
    const claude = status();
    const codex = status({
      agent_name: "codex-acp",
      display_name: "Codex",
      runtime_id: "npm:@example/codex",
      package: CODEX_PACKAGE,
      source: CODEX_PACKAGE,
    });
    updateAgentMock.mockResolvedValueOnce({
      job_id: CLAUDE_JOB,
      agent_name: AGENT_NAME,
      status: "queued",
      started_at: STARTED_AT,
    });
    render(
      <Harness
        statuses={[claude, codex]}
        onPreview={(name) =>
          Promise.resolve(
            preview(name, "1.1.0", name === AGENT_NAME ? CLAUDE_PACKAGE : CODEX_PACKAGE),
          )
        }
      />,
    );
    const claudeCheckbox = await screen.findByTestId("runtime-bulk-select-claude-acp");
    await screen.findByTestId("runtime-bulk-select-codex-acp");
    await waitFor(() => expect((claudeCheckbox as HTMLButtonElement).disabled).toBe(false));
    expect(screen.getByTestId(BULK_APPROVE).textContent).toContain("Update all (2)");

    fireEvent.click(claudeCheckbox);
    await waitFor(() =>
      expect(screen.getByTestId(BULK_APPROVE).textContent).toContain("Update selected (1)"),
    );
    fireEvent.click(screen.getByTestId(BULK_APPROVE));

    await waitFor(() => expect(updateAgentMock).toHaveBeenCalledOnce());
    expect(updateAgentMock.mock.calls[0]?.[1]).toMatchObject({
      update_mode: "pinned",
      target_version: "1.1.0",
      reviewed_runtime: expect.objectContaining({ runtime_id: "npm:@example/codex" }),
    });
    expect(await screen.findByTestId("runtime-batch-row-codex-acp")).toBeTruthy();
  });

  it("reports a partial failure and returns only failed targets to retry review", async () => {
    updateAgentMock.mockRejectedValueOnce(
      new ApiError("changed", 409, { code: "runtime_review_changed" }),
    );
    render(<Harness statuses={[status()]} onPreview={(name) => Promise.resolve(preview(name))} />);
    const approve = await screen.findByTestId(BULK_APPROVE);
    await waitFor(() => expect((approve as HTMLButtonElement).disabled).toBe(false));
    fireEvent.click(approve);
    const retry = await screen.findByTestId("runtime-batch-retry-failed");
    expect(screen.getByTestId("runtime-batch-summary").textContent).toContain("1 failed");
    fireEvent.click(retry);
    await waitFor(() => expect(screen.getByTestId(BULK_APPROVE)).toBeTruthy());
    expect(screen.getByTestId(BULK_APPROVE).textContent).toContain("Update all (1)");
  });
});

describe("AgentRuntimeUpdateManager individual entry review", () => {
  it("routes a self-update row through detail review without a POST", async () => {
    const startUpdate = vi.fn();
    const omp = status({
      agent_name: "omp-acp",
      display_name: "OMP",
      runtime_id: "omp-acp",
      owner: "external",
      mechanism: "native",
      management: "manual",
      update_mode: "self_update",
      package: "@oh-my-pi/pi-coding-agent",
      source: "@oh-my-pi/pi-coding-agent",
      auto_update_supported: false,
      latest_version: "18.3.2",
    });
    render(
      <Harness
        statuses={[omp]}
        onPreview={(name) =>
          Promise.resolve({
            ...preview(name),
            update_mode: "self_update",
            target_version: "",
            stable_latest_version: "18.3.2",
            operation: "update",
          })
        }
        onStartUpdate={startUpdate}
      />,
    );
    await screen.findByTestId("runtime-update-omp-acp");
    fireEvent.click(screen.getByTestId("runtime-update-omp-acp"));
    await waitFor(() => expect(screen.getByTestId("agent-update-dialog-omp-acp")).toBeTruthy());
    expect(startUpdate).not.toHaveBeenCalled();
    expect(updateAgentMock).not.toHaveBeenCalled();
  });

  it("opens detail review when the exact preview fails and keeps approval disabled", async () => {
    const startUpdate = vi.fn();
    render(
      <Harness
        statuses={[status()]}
        onPreview={() => Promise.reject(new Error("preview failed"))}
        onStartUpdate={startUpdate}
      />,
    );
    await screen.findByTestId("runtime-update-claude-acp");
    fireEvent.click(screen.getByTestId("runtime-update-claude-acp"));
    const confirm = await screen.findByTestId("agent-update-confirm-claude-acp");
    await waitFor(() => expect((confirm as HTMLButtonElement).disabled).toBe(true));
    expect(screen.getByText("preview failed")).toBeTruthy();
    expect(startUpdate).not.toHaveBeenCalled();
    expect(updateAgentMock).not.toHaveBeenCalled();
  });
});
