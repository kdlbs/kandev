import { cleanup, render, screen } from "@testing-library/react";
import type { ReactNode } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";

import type { Agent, AgentDiscovery } from "@/lib/types/http";

vi.mock("@/components/routing/app-link", () => ({
  default: ({ children, ...props }: { children?: ReactNode }) => <a {...props}>{children}</a>,
}));

vi.mock("@/components/agent-logo", () => ({
  AgentLogo: () => <span data-testid="agent-logo" />,
}));

vi.mock("@/components/settings/host-shell-dialog", () => ({
  HostShellDialog: () => null,
}));

vi.mock("@/components/settings/agent-login-dialog", () => ({
  AgentLoginDialog: () => null,
}));

vi.mock("@/components/settings/agent-runtime-update-control", () => ({
  AgentRuntimeUpdateControl: () => null,
}));

import { InstalledAgentCard } from "./installed-agent-card";

const DISCOVERY = {
  name: "mock-agent",
  supports_mcp: false,
  available: true,
  matched_path: null,
  login_command: undefined,
} as unknown as AgentDiscovery;

const SAVED_WITHOUT_PROFILES = {
  id: "agent-1",
  name: "mock-agent",
  profiles: [],
} as unknown as Agent;

function renderCard(savedAgent?: Agent) {
  return render(
    <InstalledAgentCard agent={DISCOVERY} savedAgent={savedAgent} displayName="Mock Agent" />,
  );
}

afterEach(() => cleanup());

describe("InstalledAgentCard setup links", () => {
  it("uses the base setup route when no agent record exists", () => {
    renderCard();

    expect(screen.getByTestId("setup-profile-mock-agent").getAttribute("href")).toBe(
      "/settings/agents/mock-agent",
    );
  });

  it("uses create mode when a saved agent has no profiles", () => {
    renderCard(SAVED_WITHOUT_PROFILES);

    expect(screen.getByTestId("setup-profile-mock-agent").getAttribute("href")).toBe(
      "/settings/agents/mock-agent?mode=create",
    );
  });
});
