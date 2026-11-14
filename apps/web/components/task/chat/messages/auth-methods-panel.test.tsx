import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { RecoveryAuthMethod } from "@/components/task/chat/types";
import { TooltipProvider } from "@kandev/ui/tooltip";
import { AuthMethodsPanel, GenericAuthPanel } from "./auth-methods-panel";

afterEach(cleanup);

describe("authentication method guidance", () => {
  it("keeps protocol-only methods separate from shell commands", () => {
    render(<AuthMethodsPanel methods={[{ id: "cursor_login", name: "Cursor CLI login" }]} />);

    expect(screen.getByText("Cursor CLI login")).toBeTruthy();
    expect(screen.getByText("cursor_login")).toBeTruthy();
    expect(screen.queryByRole("button", { name: "Run in terminal" })).toBeNull();
  });

  it("keeps a structured terminal command copyable without launching the Kandev host shell", () => {
    render(
      <TooltipProvider>
        <AuthMethodsPanel
          methods={[
            {
              id: "terminal_login",
              name: "Provider login",
              terminal_auth: { command: "provider", args: ["login"] },
            } satisfies RecoveryAuthMethod,
          ]}
        />
      </TooltipProvider>,
    );

    expect(screen.getByText("provider login")).toBeTruthy();
    expect(screen.getByRole("button", { name: "Copy command" })).toBeTruthy();
    expect(screen.queryByRole("button", { name: "Run in terminal" })).toBeNull();
    expect(
      screen.getByText("Sign in to the provider on the execution host, then retry."),
    ).toBeTruthy();
  });

  it("can omit the terminal action when the failed execution is no longer owned", () => {
    render(<GenericAuthPanel allowTerminal={false} onOpenTerminal={vi.fn()} />);
    expect(screen.queryByRole("button", { name: "Open terminal" })).toBeNull();
    expect(
      screen.getByText("Authentication required; please log in via the terminal"),
    ).toBeTruthy();
  });
});
