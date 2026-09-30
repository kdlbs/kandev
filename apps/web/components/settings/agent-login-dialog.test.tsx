import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { AgentLoginDialog } from "./agent-login-dialog";

const { refresh } = vi.hoisted(() => ({ refresh: vi.fn() }));
vi.mock("@/lib/api/domains/settings-api", () => ({ fetchDynamicModels: refresh }));
vi.mock("@/lib/api", () => ({ startAgentLogin: vi.fn() }));
vi.mock("@/components/settings/pty-terminal-dialog", () => ({
  PtyTerminalDialog: ({ onDone }: { onDone: () => void }) => <button onClick={onDone}>Done</button>,
}));

afterEach(cleanup);
beforeEach(() => refresh.mockReset());

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
      />,
    );
    fireEvent.click(screen.getByText("Done"));
    await waitFor(() => expect(rescan).toHaveBeenCalledTimes(1));
    expect(refresh).toHaveBeenCalledTimes(1);
    expect(refresh).toHaveBeenCalledWith("minimax-acp", { refresh: true });
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
