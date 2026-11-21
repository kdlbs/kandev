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
  refreshRejects = false,
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
  refreshRejects?: boolean;
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
            refresh={() =>
              refreshRejects ? Promise.reject(new Error("offline")) : Promise.resolve(refreshResult)
            }
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

describe("AgentRuntimeUpdateManager refresh gate", () => {
  it("disables bulk approval after a failed required refresh", async () => {
    render(
      <Harness
        statuses={[status()]}
        onPreview={(name) => Promise.resolve(preview(name))}
        refreshResult={false}
      />,
    );
    const approve = await screen.findByTestId(BULK_APPROVE);
    await waitFor(() => expect((approve as HTMLButtonElement).disabled).toBe(false));
    fireEvent.click(screen.getByTestId("runtime-manager-refresh"));
    await waitFor(() => expect((approve as HTMLButtonElement).disabled).toBe(true));
    expect(screen.getByText(/Could not refresh runtime information/)).toBeTruthy();
    fireEvent.click(approve);
    expect(updateAgentMock).not.toHaveBeenCalled();
  });
});

describe("AgentRuntimeUpdateManager completed batch", () => {
  it("returns a completed batch to a fresh review from the explicit action", async () => {
    updateAgentMock.mockResolvedValueOnce({
      job_id: CLAUDE_JOB,
      agent_name: AGENT_NAME,
      status: "succeeded",
      operation: "update",
      started_at: STARTED_AT,
    });
    render(<Harness statuses={[status()]} onPreview={(name) => Promise.resolve(preview(name))} />);
    const approve = await screen.findByTestId(BULK_APPROVE);
    await waitFor(() => expect((approve as HTMLButtonElement).disabled).toBe(false));
    fireEvent.click(approve);
    const reviewAgain = await screen.findByTestId("runtime-batch-review-again");
    fireEvent.click(reviewAgain);
    await waitFor(() => expect(screen.getByTestId(BULK_APPROVE)).toBeTruthy());
  });
});

describe("AgentRuntimeUpdateManager read-only details", () => {
  it("opens details for an unsupported runtime with no guidance URL and retains its outcome", async () => {
    const unsupported = status({
      agent_name: "custom-acp",
      display_name: "Custom",
      runtime_id: "custom-acp",
      owner: "none",
      mechanism: "custom",
      management: "unsupported",
      package: "",
      source: "",
      available: false,
      enabled: true,
      auto_update_supported: false,
      latest_version: undefined,
      check_state: "up_to_date",
      last_outcome: {
        id: "outcome-1",
        status: "interrupted",
        previous_version: "1.0.0",
        target_version: "1.1.0",
        finished_at: STARTED_AT,
      },
    });
    render(
      <Harness statuses={[unsupported]} onPreview={(name) => Promise.resolve(preview(name))} />,
    );
    const details = await screen.findByTestId("runtime-details-custom-acp");
    fireEvent.click(details);
    const policy = await screen.findByTestId("runtime-policy-detail-custom-acp");
    expect(policy.textContent).toContain("could not be confirmed");
    expect(policy.querySelector('a[href*="http"]')).toBeNull();
  });
});

describe("AgentRuntimeUpdateManager overview state", () => {
  it("restores scroll position and disclosure state after returning from details", async () => {
    const external = status({
      agent_name: "kimi-acp",
      display_name: "Kimi",
      runtime_id: "kimi-acp",
      owner: "external",
      mechanism: "native",
      management: "manual",
      package: "",
      source: "",
      available: true,
      enabled: true,
      auto_update_supported: false,
      latest_version: "2.0.0",
      check_state: "update_available",
    });
    render(
      <Harness
        statuses={[status(), external]}
        onPreview={(name) => Promise.resolve(preview(name))}
      />,
    );
    const body = await screen.findByTestId("runtime-manager-body");
    body.scrollTop = 240;
    fireEvent.scroll(body);
    const group = screen.getByText(/Managed externally/).closest("details")!;
    fireEvent.click(group.querySelector("summary")!);
    await waitFor(() => expect(group.open).toBe(true));

    fireEvent.click(screen.getByTestId(CLAUDE_MANAGE));
    await waitFor(() => expect(screen.getByTestId(CLAUDE_DETAIL)).toBeTruthy());
    fireEvent.click(screen.getByTestId("agent-update-back-claude-acp"));

    const restored = await screen.findByTestId("runtime-manager-body");
    expect(restored.scrollTop).toBe(240);
    const restoredGroup = screen.getByText(/Managed externally/).closest("details")!;
    expect(restoredGroup.open).toBe(true);
  });
});

describe("AgentRuntimeUpdateManager retry freshness", () => {
  it("reloads changed targets and evidence before retrying a rejected row", async () => {
    let changed = false;
    const initial = status();
    const updated = status({
      effective_version: "1.1.0",
      current_version: "1.1.0",
      active_version: "1.1.0",
      latest_version: "1.2.0",
    });
    updateAgentMock
      .mockRejectedValueOnce(new ApiError("changed", 409, { code: "runtime_review_changed" }))
      .mockResolvedValueOnce({
        job_id: CLAUDE_JOB,
        agent_name: AGENT_NAME,
        status: "queued",
        started_at: STARTED_AT,
      });
    const onPreview: PreviewFn = (name) =>
      Promise.resolve(
        changed
          ? {
              ...preview(name, "1.2.0"),
              current_version: "1.1.0",
              active_version: "1.1.0",
              effective_version: "1.1.0",
              reviewed_runtime: {
                runtime_id: "npm:@example/claude",
                package: CLAUDE_PACKAGE,
                current_version: "1.1.0",
                effective_version: "1.1.0",
                active_version: "1.1.0",
              },
            }
          : preview(name, "1.1.0"),
      );
    const { rerender } = render(<Harness statuses={[initial]} onPreview={onPreview} />);
    const approve = await screen.findByTestId(BULK_APPROVE);
    await waitFor(() => expect((approve as HTMLButtonElement).disabled).toBe(false));
    fireEvent.click(approve);
    const retry = await screen.findByTestId("runtime-batch-retry-failed");

    changed = true;
    rerender(<Harness statuses={[updated]} onPreview={onPreview} />);
    fireEvent.click(retry);
    await waitFor(() => expect(screen.getByText(/1\.1\.0 → 1\.2\.0/)).toBeTruthy());
    const retryApprove = await screen.findByTestId(BULK_APPROVE);
    await waitFor(() => expect((retryApprove as HTMLButtonElement).disabled).toBe(false));
    fireEvent.click(retryApprove);

    await waitFor(() => expect(updateAgentMock).toHaveBeenCalledTimes(2));
    expect(updateAgentMock.mock.calls[1]?.[1]).toMatchObject({
      target_version: "1.2.0",
      reviewed_runtime: expect.objectContaining({
        active_version: "1.1.0",
        effective_version: "1.1.0",
      }),
    });
  });
});

function ReopenHarness({ onPreview }: { onPreview: PreviewFn }) {
  const [open, setOpen] = useState(true);
  return (
    <StateProvider>
      <SettingsSaveProvider>
        <RuntimePolicyControllers statuses={[status()]}>
          {open ? (
            <AgentRuntimeUpdateManager
              view={{ kind: "overview" }}
              isMobile={false}
              statuses={[status()]}
              statusLoading={false}
              refreshFailed={false}
              updateJobs={{}}
              installJobs={{}}
              canManage
              hasRuntimeControl={() => true}
              resolveDisplayName={() => "Claude"}
              resolveRuntimeUpdate={() => runtimeUpdate}
              previewUpdate={onPreview}
              startUpdate={vi.fn()}
              refresh={() => Promise.resolve(true)}
              onClose={() => setOpen(false)}
              onBack={() => undefined}
              onOpenRuntime={() => undefined}
            />
          ) : (
            <button data-testid="reopen-manager" onClick={() => setOpen(true)}>
              reopen
            </button>
          )}
        </RuntimePolicyControllers>
      </SettingsSaveProvider>
    </StateProvider>
  );
}

describe("AgentRuntimeUpdateManager re-entry", () => {
  it("starts a fresh review after closing a completed batch", async () => {
    updateAgentMock.mockResolvedValueOnce({
      job_id: CLAUDE_JOB,
      agent_name: AGENT_NAME,
      status: "succeeded",
      operation: "update",
      started_at: STARTED_AT,
    });
    render(<ReopenHarness onPreview={(name) => Promise.resolve(preview(name))} />);
    const approve = await screen.findByTestId(BULK_APPROVE);
    await waitFor(() => expect((approve as HTMLButtonElement).disabled).toBe(false));
    fireEvent.click(approve);
    await screen.findByTestId("runtime-batch-review-again");
    fireEvent.click(screen.getByTestId("runtime-manager-close"));
    fireEvent.click(screen.getByTestId("reopen-manager"));
    await waitFor(() => expect(screen.getByTestId(BULK_APPROVE)).toBeTruthy());
    expect(screen.queryByTestId("runtime-batch-review-again")).toBeNull();
  });
});

describe("AgentRuntimeUpdateManager preview recovery", () => {
  it("keeps failed retry-preview targets retryable", async () => {
    let retry = false;
    let retryPreviewFails = true;
    const initial = preview(AGENT_NAME, "1.1.0");
    const onPreview: PreviewFn = () => {
      if (!retry) return Promise.resolve(initial);
      return retryPreviewFails
        ? Promise.reject(new Error("preview unavailable"))
        : Promise.resolve(initial);
    };
    updateAgentMock
      .mockRejectedValueOnce(new ApiError("changed", 409, { code: "runtime_review_changed" }))
      .mockResolvedValueOnce({
        job_id: CLAUDE_JOB,
        agent_name: AGENT_NAME,
        status: "queued",
        started_at: STARTED_AT,
      });
    render(<Harness statuses={[status()]} onPreview={onPreview} />);
    const approve = await screen.findByTestId(BULK_APPROVE);
    await waitFor(() => expect((approve as HTMLButtonElement).disabled).toBe(false));
    fireEvent.click(approve);
    const retryButton = await screen.findByTestId("runtime-batch-retry-failed");
    retry = true;
    fireEvent.click(retryButton);
    const retryPreview = await screen.findByTestId("runtime-bulk-retry-preview");
    await waitFor(() =>
      expect((screen.getByTestId(BULK_APPROVE) as HTMLButtonElement).disabled).toBe(true),
    );
    retryPreviewFails = false;
    fireEvent.click(retryPreview);
    const reapprove = await screen.findByTestId(BULK_APPROVE);
    await waitFor(() => expect((reapprove as HTMLButtonElement).disabled).toBe(false));
    fireEvent.click(reapprove);
    await waitFor(() => expect(updateAgentMock).toHaveBeenCalledTimes(2));
  });

  it("clears a selected runtime when its reviewed target changes", async () => {
    const claude = status();
    const codex = status({
      agent_name: "codex-acp",
      display_name: "Codex",
      runtime_id: "npm:@example/codex",
      package: CODEX_PACKAGE,
      source: CODEX_PACKAGE,
    });
    const updated = status({
      effective_version: "1.1.0",
      current_version: "1.1.0",
      active_version: "1.1.0",
      latest_version: "1.2.0",
    });
    let claudeTarget = "1.1.0";
    const onPreview: PreviewFn = (name) =>
      Promise.resolve(
        name === AGENT_NAME && claudeTarget === "1.2.0"
          ? {
              ...preview(name, "1.2.0"),
              current_version: "1.1.0",
              active_version: "1.1.0",
              effective_version: "1.1.0",
              reviewed_runtime: {
                runtime_id: `npm:${CLAUDE_PACKAGE}`,
                package: CLAUDE_PACKAGE,
                current_version: "1.1.0",
                effective_version: "1.1.0",
                active_version: "1.1.0",
              },
            }
          : preview(name, "1.1.0", name === AGENT_NAME ? CLAUDE_PACKAGE : CODEX_PACKAGE),
      );
    const { rerender } = render(<Harness statuses={[claude, codex]} onPreview={onPreview} />);
    const codexCheckbox = await screen.findByTestId("runtime-bulk-select-codex-acp");
    await waitFor(() => expect((codexCheckbox as HTMLButtonElement).disabled).toBe(false));
    fireEvent.click(codexCheckbox);
    claudeTarget = "1.2.0";
    rerender(<Harness statuses={[updated, codex]} onPreview={onPreview} />);
    await waitFor(() =>
      expect(screen.getByTestId("runtime-bulk-select-claude-acp").getAttribute("data-state")).toBe(
        "unchecked",
      ),
    );
    expect((screen.getByTestId(BULK_APPROVE) as HTMLButtonElement).disabled).toBe(true);
  });
});

describe("AgentRuntimeUpdateManager missing status", () => {
  it("lets a supported card open preview-driven details before status resolves", async () => {
    const onPreview = vi.fn((name: string) => Promise.resolve(preview(name)));
    render(
      <Harness
        statuses={[]}
        onPreview={onPreview}
        initialView={{ kind: "runtime", agentName: AGENT_NAME }}
      />,
    );
    await waitFor(() => expect(onPreview).toHaveBeenCalled());
    expect(screen.getByTestId(CLAUDE_DETAIL)).toBeTruthy();
    expect(screen.queryByTestId("runtime-guidance-dialog-claude-acp")).toBeNull();
  });
});

describe("AgentRuntimeUpdateManager refresh rejection", () => {
  it("treats a rejected refresh as a failed required check", async () => {
    render(
      <Harness
        statuses={[status()]}
        onPreview={(name) => Promise.resolve(preview(name))}
        refreshRejects
      />,
    );
    const approve = await screen.findByTestId(BULK_APPROVE);
    await waitFor(() => expect((approve as HTMLButtonElement).disabled).toBe(false));
    fireEvent.click(screen.getByTestId("runtime-manager-refresh"));
    await waitFor(() => expect((approve as HTMLButtonElement).disabled).toBe(true));
    expect(screen.getByText(/Could not refresh runtime information/)).toBeTruthy();
  });

  it("disables bulk approval while an exclusive update is in flight", async () => {
    const pending = Promise.withResolvers<import("@/lib/api").AgentUpdateJob>();
    const startUpdate = vi.fn(() => pending.promise);
    render(
      <Harness
        statuses={[status()]}
        onPreview={(name) => Promise.resolve(preview(name))}
        onStartUpdate={startUpdate}
      />,
    );
    const approve = await screen.findByTestId(BULK_APPROVE);
    await waitFor(() => expect((approve as HTMLButtonElement).disabled).toBe(false));
    fireEvent.click(screen.getByTestId(CLAUDE_MANAGE));
    const confirm = await screen.findByTestId("agent-update-confirm-claude-acp");
    await waitFor(() => expect((confirm as HTMLButtonElement).disabled).toBe(false));
    fireEvent.click(confirm);
    fireEvent.click(screen.getByTestId("agent-update-back-claude-acp"));
    await waitFor(() =>
      expect((screen.getByTestId(BULK_APPROVE) as HTMLButtonElement).disabled).toBe(true),
    );
    pending.resolve({
      job_id: CLAUDE_JOB,
      agent_name: AGENT_NAME,
      status: "queued",
      started_at: STARTED_AT,
    });
  });
});

describe("AgentRuntimeUpdateManager preview deduplication", () => {
  it("does not start another preview load when a mid-load render repeats the candidates", async () => {
    const pending = Promise.withResolvers<AgentUpdatePreview>();
    const onPreview = vi.fn(() => pending.promise);
    const view = render(<Harness statuses={[status()]} onPreview={onPreview} />);
    await waitFor(() => expect(onPreview).toHaveBeenCalledTimes(1));

    view.rerender(<Harness statuses={[status()]} onPreview={onPreview} />);
    await Promise.resolve();
    expect(onPreview).toHaveBeenCalledTimes(1);

    await act(async () => {
      pending.resolve(preview(AGENT_NAME));
    });
  });
});
