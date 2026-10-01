import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { ApiError } from "@/lib/api/client";
import { AgentLoginDialog } from "./agent-login-dialog";

const { refresh, start, terminal } = vi.hoisted(() => ({
  refresh: vi.fn(),
  start: vi.fn(),
  terminal: vi.fn(),
}));
vi.mock("@/lib/api/domains/settings-api", () => ({ fetchDynamicModels: refresh }));
vi.mock("@/lib/api", () => ({ startAgentLogin: start }));
vi.mock("@/components/settings/pty-terminal-dialog", () => ({
  PtyTerminalDialog: (props: { onDone: () => void }) => {
    terminal(props);
    return <button onClick={props.onDone}>Done</button>;
  },
}));

afterEach(cleanup);
beforeEach(() => {
  refresh.mockReset();
  start.mockReset();
  terminal.mockReset();
});

describe("MiniMax login completion", () => {
  it("refreshes native models before rescanning the settings cards", async () => {
    let finish!: (value: unknown) => void;
    refresh.mockReturnValueOnce(
      new Promise((resolve) => {
        finish = resolve;
      }),
    );
    const rescan = vi.fn();
    render(
      <AgentLoginDialog
        open
        onOpenChange={vi.fn()}
        agentName="minimax-acp"
        onLoginSuccess={rescan}
        refreshModelsOnDone
      />,
    );
    fireEvent.click(screen.getByText("Done"));
    expect(refresh).toHaveBeenCalledWith("minimax-acp", { refresh: true });
    expect(rescan).not.toHaveBeenCalled();
    finish({ status: "ok" });
    await waitFor(() => expect(rescan).toHaveBeenCalledTimes(1));
  });

  it("rescans after a failed refresh without selecting another agent", async () => {
    refresh.mockRejectedValueOnce(new Error("probe unavailable"));
    const rescan = vi.fn();
    render(
      <AgentLoginDialog
        open
        onOpenChange={vi.fn()}
        agentName="minimax-acp"
        onLoginSuccess={rescan}
        refreshModelsOnDone
      />,
    );
    fireEvent.click(screen.getByText("Done"));
    await waitFor(() => expect(rescan).toHaveBeenCalledTimes(1));
    expect(refresh).toHaveBeenCalledTimes(1);
    expect(refresh).toHaveBeenCalledWith("minimax-acp", { refresh: true });
  });

  it("lets the profile callback own its single capability refresh", async () => {
    refresh.mockResolvedValue({ status: "ok" });
    const profileRefresh = vi.fn(() => refresh("minimax-acp", { refresh: true }));
    render(
      <AgentLoginDialog
        open
        onOpenChange={vi.fn()}
        agentName="minimax-acp"
        onLoginSuccess={profileRefresh}
      />,
    );
    fireEvent.click(screen.getByText("Done"));
    await waitFor(() => expect(profileRefresh).toHaveBeenCalledTimes(1));
    expect(refresh).toHaveBeenCalledTimes(1);
  });

  it("retains other agents' login completion behavior", () => {
    const rescan = vi.fn();
    render(
      <AgentLoginDialog open onOpenChange={vi.fn()} agentName="claude" onLoginSuccess={rescan} />,
    );
    fireEvent.click(screen.getByText("Done"));
    expect(rescan).toHaveBeenCalledTimes(1);
    expect(refresh).not.toHaveBeenCalled();
  });
});

describe("native login command ownership", () => {
  it("localizes a command conflict without retrying or replacing the original session", async () => {
    start.mockRejectedValue(
      new ApiError("login_command_conflict", 409, { error_code: "login_command_conflict" }),
    );
    render(<AgentLoginDialog open onOpenChange={vi.fn()} agentName="minimax-acp" />);
    const { startSession } = terminal.mock.calls.at(-1)![0];
    await expect(startSession({ cols: 80, rows: 24 })).rejects.toThrow(
      "Another sign-in command is already running. Close that terminal before trying again.",
    );
    expect(start).toHaveBeenCalledTimes(1);
  });

  it("preserves unrelated start failures without retrying", async () => {
    const unavailable = new ApiError("agent not found", 404, { error: "agent not found" });
    start.mockRejectedValue(unavailable);
    render(<AgentLoginDialog open onOpenChange={vi.fn()} agentName="minimax-acp" />);
    const { startSession } = terminal.mock.calls.at(-1)![0];
    await expect(startSession({ cols: 80, rows: 24 })).rejects.toBe(unavailable);
    expect(start).toHaveBeenCalledTimes(1);
  });
});
