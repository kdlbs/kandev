import { afterEach, describe, expect, it, vi } from "vitest";
import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { StateProvider } from "@/components/state-provider";
import { ApiError } from "@/lib/api/client";
import { updateAgentStatus } from "@/lib/api/domains/office-api";
import { defaultOfficeState } from "@/lib/state/slices/office/office-slice";
import type { AgentProfile } from "@/lib/state/slices/office/types";
import { AgentRecoveryControl } from "./agent-recovery-control";

vi.mock("sonner", () => ({ toast: { success: vi.fn(), error: vi.fn() } }));
vi.mock("@/lib/api/domains/office-api", async () => {
  const actual = await vi.importActual<typeof import("@/lib/api/domains/office-api")>(
    "@/lib/api/domains/office-api",
  );
  return {
    ...actual,
    updateAgentStatus: vi.fn(),
  };
});

const pointerMode = vi.hoisted(() => ({ isFinePointer: true }));
vi.mock("@/hooks/use-responsive-breakpoint", () => ({
  useResponsiveBreakpoint: () => ({ isFinePointer: pointerMode.isFinePointer }),
}));

afterEach(() => {
  cleanup();
  vi.clearAllMocks();
  pointerMode.isFinePointer = true;
});

const WORKSPACE_ID = "ws-1";
const AGENT_ID = "agent-1";
const TIMESTAMP = "2026-05-04T00:00:00Z";
const CONTROL_TESTID = "agent-recovery-control";
const PAUSE_REASON_TESTID = "agent-pause-reason";
const PAUSE_REASON_TEXT = "Auto-paused: too many consecutive failures";

const baseAgent: AgentProfile = {
  id: AGENT_ID,
  workspaceId: WORKSPACE_ID,
  name: "Worker",
  role: "worker",
  status: "paused",
  pauseReason: PAUSE_REASON_TEXT,
  agentProfileId: "profile-1",
  createdAt: TIMESTAMP,
  updatedAt: TIMESTAMP,
  permissions: {},
  budgetMonthlyCents: 0,
  maxConcurrentSessions: 1,
} as AgentProfile;

function renderControl(agent: AgentProfile, agents: AgentProfile[] = [agent]) {
  return render(
    <StateProvider
      initialState={{
        workspaces: { activeId: WORKSPACE_ID, items: [] },
        office: {
          ...defaultOfficeState.office,
          agentProfilesByWorkspaceId: { [WORKSPACE_ID]: agents },
        },
      }}
    >
      <AgentRecoveryControl agentId={agent.id} />
    </StateProvider>,
  );
}

function getControlButton() {
  return screen.getByTestId(CONTROL_TESTID) as HTMLButtonElement;
}

describe("AgentRecoveryControl visibility", () => {
  it("renders the control for a paused agent", () => {
    renderControl(baseAgent);
    expect(screen.getByTestId(CONTROL_TESTID)).toBeTruthy();
  });

  it("renders the control for a stopped agent", () => {
    renderControl({ ...baseAgent, status: "stopped" });
    expect(screen.getByTestId(CONTROL_TESTID)).toBeTruthy();
  });

  it.each(["idle", "working", "pending_approval", "", "some-unknown-status"])(
    "does not render the control for status %s",
    (status) => {
      renderControl({ ...baseAgent, status: status as AgentProfile["status"] });
      expect(screen.queryByTestId(CONTROL_TESTID)).toBeNull();
    },
  );

  it("shows the pause reason text when non-empty and status is recoverable", () => {
    renderControl(baseAgent);
    expect(screen.getByTestId(PAUSE_REASON_TESTID).textContent).toBe(PAUSE_REASON_TEXT);
  });

  it("shows no pause reason text when pause_reason is empty", () => {
    renderControl({ ...baseAgent, pauseReason: "" });
    expect(screen.queryByTestId(PAUSE_REASON_TESTID)).toBeNull();
  });

  it("shows no pause reason text when status is not recoverable, even if set", () => {
    renderControl({ ...baseAgent, status: "idle", pauseReason: "stale text" });
    expect(screen.queryByTestId(PAUSE_REASON_TESTID)).toBeNull();
  });

  it("carries one accessible name, not 'Resume' or 'Reactivate' alone, for both statuses", () => {
    const { unmount } = renderControl(baseAgent);
    const pausedName = getControlButton().textContent;
    expect(pausedName).toBeTruthy();
    expect(pausedName?.trim().toLowerCase()).not.toBe("resume");
    expect(pausedName?.trim().toLowerCase()).not.toBe("reactivate");
    unmount();

    renderControl({ ...baseAgent, status: "stopped" });
    expect(getControlButton().textContent).toBe(pausedName);
  });

  it("uses a coarse-pointer-sized (44px) hit area on a touch viewport", () => {
    pointerMode.isFinePointer = false;
    renderControl(baseAgent);
    expect(getControlButton().className).toContain("min-h-11");
  });

  it("keeps the compact desktop button density on a fine-pointer viewport", () => {
    pointerMode.isFinePointer = true;
    renderControl(baseAgent);
    expect(getControlButton().className).not.toContain("min-h-11");
  });
});

describe("AgentRecoveryControl activation", () => {
  it("requests only the constant target idle and no other field", async () => {
    vi.mocked(updateAgentStatus).mockResolvedValueOnce({
      ...baseAgent,
      status: "idle",
      pauseReason: "",
    });
    renderControl(baseAgent);

    fireEvent.click(getControlButton());

    await waitFor(() => {
      expect(updateAgentStatus).toHaveBeenCalledWith(AGENT_ID, "idle");
    });
    expect(updateAgentStatus).toHaveBeenCalledTimes(1);
  });

  it("disables the control and indicates in-flight while the request is pending", async () => {
    let resolveRequest: (value: AgentProfile) => void = () => {};
    vi.mocked(updateAgentStatus).mockReturnValueOnce(
      new Promise((resolve) => {
        resolveRequest = resolve;
      }),
    );
    renderControl(baseAgent);

    const button = getControlButton();
    fireEvent.click(button);

    await waitFor(() => expect(button.disabled).toBe(true));
    fireEvent.click(button);
    expect(updateAgentStatus).toHaveBeenCalledTimes(1);

    // On success the agent is idle, so the control disappears entirely
    // rather than re-enabling.
    resolveRequest({ ...baseAgent, status: "idle", pauseReason: "" });
    await waitFor(() => {
      expect(screen.queryByTestId(CONTROL_TESTID)).toBeNull();
    });
  });

  it("on success, renders the server-returned status and stops showing the control and pause reason", async () => {
    vi.mocked(updateAgentStatus).mockResolvedValueOnce({
      ...baseAgent,
      status: "idle",
      pauseReason: "",
    });
    renderControl(baseAgent);

    fireEvent.click(getControlButton());

    await waitFor(() => {
      expect(screen.queryByTestId(CONTROL_TESTID)).toBeNull();
    });
    expect(screen.queryByTestId(PAUSE_REASON_TESTID)).toBeNull();
  });

  it("on failure, leaves the displayed status and pause reason unchanged and re-enables retry", async () => {
    vi.mocked(updateAgentStatus).mockRejectedValueOnce(
      new ApiError("agent is not in a recoverable state", 400, { error: "bad transition" }),
    );
    renderControl(baseAgent);

    const button = getControlButton();
    fireEvent.click(button);

    await waitFor(() => expect(button.disabled).toBe(false));
    expect(screen.getByTestId(CONTROL_TESTID)).toBeTruthy();
    expect(screen.getByTestId(PAUSE_REASON_TESTID).textContent).toBe(PAUSE_REASON_TEXT);

    // Retry is accepted.
    vi.mocked(updateAgentStatus).mockResolvedValueOnce({
      ...baseAgent,
      status: "idle",
      pauseReason: "",
    });
    fireEvent.click(button);
    await waitFor(() => {
      expect(screen.queryByTestId(CONTROL_TESTID)).toBeNull();
    });
  });
});
