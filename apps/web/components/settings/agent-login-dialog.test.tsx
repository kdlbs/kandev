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
  PtyTerminalDialog: (props: { onDone: () => void; description?: string }) => {
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

  it("starts Antigravity CLI sign-in with the registered runtime variant", async () => {
    start.mockResolvedValue({ session_id: "login-session" });
    render(
      <AgentLoginDialog
        open
        onOpenChange={vi.fn()}
        agentName="antigravity-acp"
        runtimeVariant="cli"
        command={["/host/agy", "acp", "--login"]}
        variants={{ "antigravity-cli": ["/host/agy", "acp", "--login"] }}
      />,
    );
    expect(terminal.mock.calls.at(-1)![0].description).toContain("agy acp --login");
    const { startSession } = terminal.mock.calls.at(-1)![0];
    await startSession({ cols: 80, rows: 24 });
    expect(start).toHaveBeenCalledWith(
      "antigravity-acp",
      { cols: 80, rows: 24 },
      { commandVariant: "antigravity-cli" },
    );
    expect(terminal.mock.calls.at(-1)![0].command).toEqual(["/host/agy", "acp", "--login"]);
  });

  it("explains when the selected Antigravity runtime changed before login", async () => {
    start.mockRejectedValue(
      new ApiError("login_runtime_changed", 409, { error_code: "login_runtime_changed" }),
    );
    render(
      <AgentLoginDialog
        open
        onOpenChange={vi.fn()}
        agentName="antigravity-acp"
        runtimeVariant="cli"
        command={["/host/agy", "acp", "--login"]}
        variants={{ "antigravity-cli": ["/host/agy", "acp", "--login"] }}
      />,
    );
    const { startSession } = terminal.mock.calls.at(-1)![0];
    await expect(startSession({ cols: 80, rows: 24 })).rejects.toThrow(
      "The selected Antigravity runtime changed. Refresh agent status and try again.",
    );
    expect(start).toHaveBeenCalledTimes(1);
  });
});
