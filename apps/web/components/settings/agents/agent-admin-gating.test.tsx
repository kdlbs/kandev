import { cleanup, render, screen } from "@testing-library/react";
import type { ReactNode } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { TooltipProvider } from "@kandev/ui/tooltip";

import type { Agent, AgentProfile } from "@/lib/types/http";

// Agents and agent profiles are org configuration: every mutating route behind
// these components requires org.config.manage, which only an administrator
// holds. isAdmin is the frontend's mirror of that scope, so it is the one knob
// these cases turn.
let isAdmin = true;

vi.mock("@/hooks/domains/auth/use-is-admin", () => ({ useIsAdmin: () => isAdmin }));

const storeState = {
  settingsAgents: { items: [] as Agent[] },
  agentProfiles: { items: [] as Array<{ id: string }> },
};

vi.mock("@/components/state-provider", () => ({
  useAppStore: (selector: (s: unknown) => unknown) => selector(storeState),
  useAppStoreApi: () => ({ getState: () => storeState, setState: vi.fn() }),
}));

vi.mock("@/app/actions/agents", () => ({
  deleteAgentProfileAction: vi.fn(),
  duplicateAgentProfileAction: vi.fn(),
}));

vi.mock("@/components/toast-provider", () => ({ useToast: () => ({ toast: vi.fn() }) }));

vi.mock("@/lib/routing/client-router", () => ({
  useRouter: () => ({ push: vi.fn() }),
  usePathname: () => "/settings/agents",
}));

vi.mock("@/hooks/use-responsive-breakpoint", () => ({
  useResponsiveBreakpoint: () => ({ isFullDesktop: true, isFinePointer: true }),
}));

vi.mock("@/components/routing/app-link", () => ({
  default: ({ children, ...props }: { children?: ReactNode }) => <a {...props}>{children}</a>,
}));

vi.mock("@kandev/ui/dropdown-menu", () => ({
  DropdownMenu: ({ children }: { children?: ReactNode }) => <div>{children}</div>,
  DropdownMenuTrigger: ({ children }: { children?: ReactNode }) => <>{children}</>,
  DropdownMenuContent: ({ children }: { children?: ReactNode }) => <div>{children}</div>,
  DropdownMenuItem: ({ children }: { children?: ReactNode }) => <button>{children}</button>,
}));

import { ProfileRow } from "./agent-profiles-section";
import { InstallAgentCard } from "@/components/settings/install-agent-card";
import type { AvailableAgent } from "@/lib/types/http";

const AGENT = {
  id: "agent-1",
  name: "claude",
  profiles: [{ id: "p-1", name: "Alpha", agentDisplayName: "Claude Code" } as AgentProfile],
} as unknown as Agent;

const AVAILABLE = {
  name: "claude",
  display_name: "Claude Code",
  install_script: "npm i -g claude",
} as unknown as AvailableAgent;

afterEach(() => {
  cleanup();
  isAdmin = true;
});

describe("agent settings write controls follow org.config.manage", () => {
  it("offers a profile's duplicate and delete actions to an administrator", () => {
    render(
      <TooltipProvider>
        <ProfileRow agent={AGENT} profile={AGENT.profiles[0]} />
      </TooltipProvider>,
    );
    expect(screen.queryByTestId("duplicate-profile-inline-p-1")).not.toBeNull();
    expect(screen.queryByTestId("delete-profile-inline-p-1")).not.toBeNull();
  });

  // The row itself stays: seeing which profiles exist is a read, and the task
  // dialog offers the same profiles. Only the writes go.
  it("withholds them from a member, keeping the row readable", () => {
    isAdmin = false;
    render(
      <TooltipProvider>
        <ProfileRow agent={AGENT} profile={AGENT.profiles[0]} />
      </TooltipProvider>,
    );
    expect(screen.getByText("Alpha")).toBeTruthy();
    expect(screen.queryByTestId("duplicate-profile-inline-p-1")).toBeNull();
    expect(screen.queryByTestId("delete-profile-inline-p-1")).toBeNull();
    expect(screen.queryByTestId("profile-actions-menu-p-1")).toBeNull();
  });

  it("renders the install button only when the caller may install", () => {
    const { rerender } = render(
      <TooltipProvider>
        <InstallAgentCard agent={AVAILABLE} job={undefined} onInstall={vi.fn()} />
      </TooltipProvider>,
    );
    expect(screen.queryByTestId("install-button-claude")).not.toBeNull();

    rerender(
      <TooltipProvider>
        <InstallAgentCard agent={AVAILABLE} job={undefined} />
      </TooltipProvider>,
    );
    expect(screen.queryByTestId("install-button-claude")).toBeNull();
  });
});
