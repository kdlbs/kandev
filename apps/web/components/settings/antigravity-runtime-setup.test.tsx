import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { AvailableAgent } from "@/lib/types/http";
import { AntigravityRuntimeLabel } from "./antigravity-runtime-label";
import { InstallAgentCard } from "./install-agent-card";
import { NoAuthPanel } from "./profile-status-panels";

const mocks = vi.hoisted(() => ({
  state: {
    availableAgents: {
      items: [
        {
          name: "antigravity-acp",
          login_command: {
            cmd: ["/host/agy", "acp", "--login"],
            variants: { "antigravity-cli": ["/host/agy", "acp", "--login"] },
          },
        },
      ],
    },
    cliLoginOpen: false,
    standaloneLoginOpen: false,
  },
}));

vi.mock("@/components/state-provider", () => ({
  useAppStore: (selector: (state: typeof mocks.state) => unknown) => selector(mocks.state),
}));

vi.mock("./agent-login-dialog", () => ({
  AgentLoginDialog: (props: { open: boolean }) => {
    mocks.state.cliLoginOpen = props.open;
    return props.open ? <div data-testid="antigravity-cli-login" /> : null;
  },
}));

vi.mock("./antigravity-login-dialog", () => ({
  AntigravityLoginDialog: (props: { open: boolean }) => {
    mocks.state.standaloneLoginOpen = props.open;
    return props.open ? <div data-testid="antigravity-standalone-login" /> : null;
  },
}));

vi.mock("./host-shell-dialog", () => ({ HostShellDialog: () => null }));

afterEach(() => {
  cleanup();
  mocks.state.cliLoginOpen = false;
  mocks.state.standaloneLoginOpen = false;
});

describe("Antigravity runtime setup", () => {
  it("labels the selected CLI and standalone runtime", () => {
    const { rerender } = render(<AntigravityRuntimeLabel variant="cli" />);
    expect(screen.getByTestId("antigravity-runtime-label").textContent).toContain(
      "Antigravity CLI",
    );

    rerender(<AntigravityRuntimeLabel variant="standalone" />);
    expect(screen.getByTestId("antigravity-runtime-label").textContent).toContain(
      "Standalone ACP server",
    );
  });

  it("opens the host CLI login terminal when the CLI is selected", () => {
    render(
      <NoAuthPanel
        agentName="antigravity-acp"
        status="auth_required"
        runtimeVariant="cli"
        isLoading={false}
        onRefresh={vi.fn()}
        error={null}
        rawError={null}
      />,
    );

    expect(screen.getByText("Google sign-in required")).toBeTruthy();
    fireEvent.click(screen.getByTestId("profile-no-auth-open-terminal"));
    expect(screen.getByTestId("antigravity-cli-login")).toBeTruthy();
    expect(mocks.state.standaloneLoginOpen).toBe(false);
  });

  it("opens standalone consent when the ACP server is selected", () => {
    render(
      <NoAuthPanel
        agentName="antigravity-acp"
        status="auth_required"
        runtimeVariant="standalone"
        isLoading={false}
        onRefresh={vi.fn()}
        error={null}
        rawError={null}
      />,
    );

    fireEvent.click(screen.getByTestId("profile-no-auth-open-terminal"));
    expect(screen.getByTestId("antigravity-standalone-login")).toBeTruthy();
    expect(mocks.state.cliLoginOpen).toBe(false);
  });

  it("shows the CLI setup guide and labels the local install as standalone", () => {
    const onInstall = vi.fn();
    render(
      <>
        <NoAuthPanel
          agentName="antigravity-acp"
          status="not_installed"
          runtimeVariant="standalone"
          isLoading={false}
          onRefresh={vi.fn()}
          error={null}
          rawError={null}
        />
        <InstallAgentCard
          agent={
            {
              name: "antigravity-acp",
              display_name: "Antigravity",
              supports_local_install: true,
            } as unknown as AvailableAgent
          }
          job={undefined}
          onInstall={onInstall}
        />
      </>,
    );

    expect(screen.getByTestId("antigravity-cli-setup-guide").getAttribute("href")).toBe(
      "https://antigravity.google/docs/cli",
    );
    fireEvent.click(screen.getByRole("button", { name: "Install standalone ACP server" }));
    expect(onInstall).toHaveBeenCalledWith("antigravity-acp");
  });
});
