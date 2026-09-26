import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { StateProvider, useAppStoreApi } from "@/components/state-provider";
import type { RestartCapabilityState } from "@/hooks/domains/system/use-restart-capability";
import type { KandevRestartController } from "@/hooks/domains/system/use-kandev-restart";
import { AgentRuntimeUnavailableAlert } from "./agent-runtime-unavailable-alert";

const RESTART_BUTTON_LABEL = "Restart Kandev";

const restartCapability = vi.hoisted(() => ({
  value: {
    status: "resolved",
    capability: { supported: true, mode: "supervisor", adapter: "supervisor" },
  } as RestartCapabilityState,
}));
const restartController = vi.hoisted(() => ({
  value: {
    phase: "idle",
    errorMessage: null,
    isRestarting: false,
    start: vi.fn(),
    dismiss: vi.fn(),
  } as KandevRestartController,
}));

vi.mock("@/hooks/domains/system/use-restart-capability", () => ({
  useRestartCapability: () => restartCapability.value,
}));

vi.mock("@/hooks/domains/system/use-kandev-restart", () => ({
  useKandevRestart: () => restartController.value,
}));

vi.mock("@/components/settings/system/restart-progress-dialog", () => ({
  RestartProgressDialog: ({ phase }: { phase: string }) =>
    phase === "idle" ? null : <div data-testid="restart-progress-dialog" data-phase={phase} />,
}));

function RuntimeStateControls() {
  const store = useAppStoreApi();
  return (
    <>
      <button
        type="button"
        data-testid="runtime-recover"
        onClick={() => store.getState().setAgentRuntime({ status: "available" })}
      />
      <button
        type="button"
        data-testid="connection-recover"
        onClick={() => {
          store.getState().setConnectionStatus("connected");
          store.getState().setConnectionIssueSeverity("none");
        }}
      />
    </>
  );
}

function renderAlert() {
  return render(
    <StateProvider
      initialState={{
        agentRuntime: {
          status: "unavailable",
          reason: "agentctl_exited",
          occurred_at: "2026-08-08T14:22:52Z",
        },
      }}
    >
      <AgentRuntimeUnavailableAlert />
      <div data-testid="retained-route-content">Last known task</div>
      <RuntimeStateControls />
    </StateProvider>,
  );
}

describe("AgentRuntimeUnavailableAlert", () => {
  beforeEach(() => {
    restartCapability.value = {
      status: "resolved",
      capability: { supported: true, mode: "supervisor", adapter: "supervisor" },
    };
    restartController.value = {
      phase: "idle",
      errorMessage: null,
      isRestarting: false,
      start: vi.fn(),
      dismiss: vi.fn(),
    };
  });

  afterEach(cleanup);

  it("renders one persistent alert with the supported restart flow", () => {
    renderAlert();

    expect(screen.getAllByRole("alert")).toHaveLength(1);
    expect(screen.getByText("Local agent runtime stopped")).toBeTruthy();
    expect(screen.getByText(/saved data remains safe/i)).toBeTruthy();
    expect(screen.getByRole("button", { name: RESTART_BUTTON_LABEL })).toBeTruthy();
    expect(screen.getByTestId("retained-route-content")).toBeTruthy();

    fireEvent.click(screen.getByRole("button", { name: RESTART_BUTTON_LABEL }));

    expect(restartController.value.start).toHaveBeenCalledOnce();
  });

  it("uses manual guidance when capability lookup is unavailable", () => {
    restartCapability.value = { status: "unavailable", capability: null };
    renderAlert();

    expect(screen.queryByRole("button", { name: RESTART_BUTTON_LABEL })).toBeNull();
    expect(screen.getByText(/terminal or service manager/i)).toBeTruthy();
  });

  it("keeps the alert through connection changes and restart errors", () => {
    restartController.value = {
      phase: "error",
      errorMessage: "restart request failed",
      isRestarting: false,
      start: vi.fn(),
      dismiss: vi.fn(),
    };
    renderAlert();

    fireEvent.click(screen.getByTestId("connection-recover"));
    expect(screen.getByRole("alert")).toBeTruthy();

    fireEvent.click(screen.getByTestId("runtime-recover"));
    expect(screen.queryByRole("alert")).toBeNull();
  });

  it("keeps the phone action at the touch-target size and stacks the content", () => {
    renderAlert();

    const alert = screen.getByRole("alert");
    const action = screen.getByRole("button", { name: RESTART_BUTTON_LABEL });
    expect(alert.className).toContain("min-w-0");
    expect(action.className).toContain("h-11");
    expect(action.className).toContain("w-full");
    expect(action.className).toContain("sm:w-auto");
  });
});
