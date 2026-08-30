import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { ReactNode } from "react";
import type { AgentProfile } from "@/lib/types/http";
import { agentProfileId } from "@/lib/types/ids";
import { DynamicAgentsCard } from "./dynamic-agents-card";

const { mockUseFeature } = vi.hoisted(() => ({
  mockUseFeature: vi.fn(() => true),
}));

vi.mock("react-i18next", () => ({
  useTranslation: () => ({
    t: (key: string) =>
      ({
        "agents:dynamicAgents": "Dynamic agents",
        "agents:dynamicAgentsDescription": "Route profiles",
        "agents:dynamicProfileDisabled": "Routing is disabled",
        "agents:dynamicCandidates": "Candidates",
        "agents:newProfile": "New profile",
        "agents:noProfilesYetShort": "No profiles yet",
        "agents:profileName": "Profile",
        "sidebar:disabledBadge": "Disabled",
      })[key] ?? key,
  }),
}));

vi.mock("@/hooks/domains/features/use-feature", () => ({
  useFeature: mockUseFeature,
}));

vi.mock("@/components/routing/app-link", () => ({
  default: ({ children, ...props }: { children?: ReactNode; href: string }) => (
    <a {...props}>{children}</a>
  ),
}));

const dynamicProfile: AgentProfile = {
  id: agentProfileId("dynamic-profile-1"),
  kind: "dynamic",
  name: "Balanced",
  agentId: "dynamic",
  agentDisplayName: "Dynamic",
  model: "",
  allowIndexing: false,
  autoApprove: false,
  cliFlags: [],
  cliPassthrough: false,
  createdAt: "2026-01-01T00:00:00Z",
  updatedAt: "2026-01-01T00:00:00Z",
  dynamic: {
    version: 1,
    candidates: [
      {
        position: 0,
        executionProfileId: agentProfileId("concrete-profile-1"),
        enabled: true,
        policies: {
          version: 1,
          transient: {
            retry: { enabled: false, maxRetries: 0, initialIntervalSeconds: 0 },
            waitForReset: { enabled: false, maxWaitSeconds: 0 },
            onExhausted: "skip",
          },
          hard: {
            retry: { enabled: false, maxRetries: 0, initialIntervalSeconds: 0 },
            waitForReset: { enabled: false, maxWaitSeconds: 0 },
            onExhausted: "skip",
          },
        },
      },
      {
        position: 1,
        executionProfileId: agentProfileId("concrete-profile-2"),
        enabled: true,
        policies: {
          version: 1,
          transient: {
            retry: { enabled: false, maxRetries: 0, initialIntervalSeconds: 0 },
            waitForReset: { enabled: false, maxWaitSeconds: 0 },
            onExhausted: "stop",
          },
          hard: {
            retry: { enabled: false, maxRetries: 0, initialIntervalSeconds: 0 },
            waitForReset: { enabled: false, maxWaitSeconds: 0 },
            onExhausted: "stop",
          },
        },
      },
    ],
  },
};

describe("DynamicAgentsCard", () => {
  afterEach(() => {
    cleanup();
    mockUseFeature.mockReturnValue(true);
  });

  it("does not render when dynamic routing is disabled", () => {
    mockUseFeature.mockReturnValue(false);

    render(<DynamicAgentsCard agent={{ profiles: [dynamicProfile] }} />);

    expect(screen.queryByTestId("dynamic-agents-card")).toBeNull();
  });

  it("offers creation and links each existing dynamic profile", () => {
    render(<DynamicAgentsCard agent={{ profiles: [dynamicProfile] }} />);

    expect(screen.getByTestId("new-dynamic-profile").getAttribute("href")).toBe(
      "/settings/agents/dynamic?mode=create",
    );
    expect(screen.getByTestId("dynamic-profile-route-dynamic-profile-1").getAttribute("href")).toBe(
      "/settings/agents/dynamic/profiles/dynamic-profile-1",
    );
    expect(screen.getByText("Balanced")).toBeTruthy();
    expect(screen.getByText("Candidates: 2")).toBeTruthy();
  });

  it("shows an empty state while keeping the creation action available", () => {
    render(<DynamicAgentsCard agent={{ profiles: [] }} />);

    expect(screen.getByTestId("dynamic-agents-empty").textContent).toContain("No profiles yet");
    expect(screen.getByTestId("new-dynamic-profile").getAttribute("href")).toBe(
      "/settings/agents/dynamic?mode=create",
    );
  });
});
