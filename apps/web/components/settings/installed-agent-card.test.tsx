import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import type { ReactNode } from "react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { makeLocalStorageMock } from "@/hooks/local-storage-mock.test-helpers";
import type { Agent, AgentDiscovery } from "@/lib/types/http";
import { InstalledAgentCard } from "./installed-agent-card";

const STORAGE_KEY = "kandev:agents:collapsedBlocks:v1";

const localStorageMock = makeLocalStorageMock();
vi.stubGlobal("localStorage", localStorageMock);
Object.defineProperty(window, "localStorage", { value: localStorageMock, configurable: true });

// Chrome around the collapse behavior: dialogs, runtime-update control, and
// identity chrome are all irrelevant to the body visibility + header count.
vi.mock("@/components/agent-logo", () => ({
  AgentLogo: () => <span data-testid="agent-logo" />,
}));
vi.mock("@/components/settings/record-badges", () => ({
  NotInstalledBadge: () => null,
}));
vi.mock("@/components/settings/agent-login-dialog", () => ({
  AgentLoginDialog: () => null,
}));
vi.mock("@/components/settings/agent-runtime-update-control", () => ({
  AgentRuntimeUpdateControl: () => null,
}));
vi.mock("@/components/settings/host-shell-dialog", () => ({
  HostShellDialog: () => null,
}));
vi.mock("@/components/routing/app-link", () => ({
  default: ({ children, ...props }: { children?: ReactNode }) => <a {...props}>{children}</a>,
}));
vi.mock("@kandev/ui/tooltip", () => ({
  Tooltip: ({ children }: { children?: ReactNode }) => <>{children}</>,
  TooltipTrigger: ({ children }: { children?: ReactNode }) => <>{children}</>,
  TooltipContent: () => null,
}));

const DISPLAY_NAME = "Claude Code";

function makeDiscovery(name: string): AgentDiscovery {
  return {
    name,
    supports_mcp: false,
    mcp_config_path: null,
    installation_paths: [],
    available: true,
    matched_path: null,
  };
}

function makeSavedAgent(name: string, profileCount: number): Agent {
  return {
    id: `agent-${name}`,
    name,
    supports_mcp: false,
    mcp_config_path: null,
    profiles: Array.from({ length: profileCount }, (_, i) => ({
      id: `p-${name}-${i}`,
      name: `Profile ${i + 1}`,
    })),
    created_at: "2026-01-01T00:00:00Z",
    updated_at: "2026-01-01T00:00:00Z",
  } as Agent;
}

function renderCard(profileCount: number) {
  const agent = makeDiscovery("claude");
  return render(
    <InstalledAgentCard
      agent={agent}
      savedAgent={makeSavedAgent("claude", profileCount)}
      displayName={DISPLAY_NAME}
    >
      <div data-testid="profiles-body">profiles</div>
    </InstalledAgentCard>,
  );
}

function renderSetupCard(savedAgent?: Agent) {
  return render(
    <InstalledAgentCard
      agent={makeDiscovery("mock-agent")}
      savedAgent={savedAgent}
      displayName="Mock Agent"
    >
      <div data-testid="profiles-body">profiles</div>
    </InstalledAgentCard>,
  );
}

const toggle = () => screen.getByTestId("collapse-agent-claude");
const toggleAriaLabel = () => toggle().getAttribute("aria-label");
const toggleAriaExpanded = () => toggle().getAttribute("aria-expanded");
const body = () => screen.queryByTestId("profiles-body");
const headerCount = () => screen.queryByTestId("collapsed-count-claude");

/** Radix Presence unmounts closed content after the exit animation; in a DOM
 *  test environment either the element is detached or it carries `hidden`.
 *  Both mean the body is not visible to the user. */
function expectBodyNotVisible() {
  const el = body();
  expect(el === null || el.hidden).toBe(true);
}

describe("InstalledAgentCard collapse", () => {
  beforeEach(() => {
    localStorageMock.clear();
  });
  afterEach(() => {
    cleanup();
  });

  it("renders the profiles body expanded by default and stores nothing", () => {
    renderCard(3);
    expect(body()).not.toBeNull();
    expect(body()!.hidden).toBe(false);
    expect(window.localStorage.getItem(STORAGE_KEY)).toBeNull();
    expect(toggleAriaExpanded()).toBe("true");
    expect(toggleAriaLabel()).toBe("Collapse Claude Code profiles");
  });

  it("collapses the body, shows the count in the header, and persists", async () => {
    renderCard(3);
    fireEvent.click(toggle());

    await waitFor(() => expectBodyNotVisible());
    expect(headerCount()?.textContent).toContain("3 profiles");
    expect(window.localStorage.getItem(STORAGE_KEY)).toBe(JSON.stringify({ claude: true }));
    expect(toggleAriaExpanded()).toBe("false");
    expect(toggleAriaLabel()).toBe("Expand Claude Code profiles");
  });

  it("mounts collapsed when a collapsed entry is stored and shows the count", async () => {
    window.localStorage.setItem(STORAGE_KEY, JSON.stringify({ claude: true }));
    renderCard(3);

    await waitFor(() => expectBodyNotVisible());
    expect(headerCount()?.textContent).toContain("3 profiles");
    expect(toggleAriaExpanded()).toBe("false");
  });

  it("expanding restores the body and removes the count from the header", async () => {
    window.localStorage.setItem(STORAGE_KEY, JSON.stringify({ claude: true }));
    renderCard(3);

    await waitFor(() => expectBodyNotVisible());
    fireEvent.click(toggle());

    await waitFor(() => expect(body()?.hidden).toBe(false));
    expect(headerCount()).toBeNull();
    expect(toggleAriaExpanded()).toBe("true");
  });

  it("keeps other agents' collapse entries from affecting this card", () => {
    window.localStorage.setItem(STORAGE_KEY, JSON.stringify({ codex: true }));
    renderCard(3);
    expect(body()).not.toBeNull();
    expect(body()!.hidden).toBe(false);
  });

  it("shows No profiles yet in the header for a collapsed zero-profile agent", async () => {
    renderCard(0);
    fireEvent.click(toggle());

    await waitFor(() => expectBodyNotVisible());
    expect(headerCount()?.textContent).toContain("No profiles yet");
  });

  it("places the count left of the collapse button while collapsed", async () => {
    renderCard(3);
    fireEvent.click(toggle());

    await waitFor(() => expectBodyNotVisible());
    const count = headerCount();
    expect(count).not.toBeNull();
    expect(
      count!.compareDocumentPosition(toggle()) & Node.DOCUMENT_POSITION_FOLLOWING,
    ).toBeTruthy();
  });

  it("applies the toggle in memory and never throws when persisting the collapse fails", () => {
    renderCard(3);
    const original = localStorageMock.setItem;
    localStorageMock.setItem = () => {
      throw new Error("quota exceeded");
    };
    try {
      expect(() => fireEvent.click(toggle())).not.toThrow();
      // The write failed, but the collapse still applies for the session.
      expectBodyNotVisible();
      expect(toggleAriaExpanded()).toBe("false");
      expect(window.localStorage.getItem(STORAGE_KEY)).toBeNull();
    } finally {
      localStorageMock.setItem = original;
    }
  });

  it("lets a collapsed card expand when persisting the change fails", async () => {
    window.localStorage.setItem(STORAGE_KEY, JSON.stringify({ claude: true }));
    renderCard(3);

    await waitFor(() => expectBodyNotVisible());
    const original = localStorageMock.setItem;
    localStorageMock.setItem = () => {
      throw new Error("quota exceeded");
    };
    try {
      expect(() => fireEvent.click(toggle())).not.toThrow();
      await waitFor(() => expect(body()?.hidden).toBe(false));
      expect(headerCount()).toBeNull();
      expect(toggleAriaExpanded()).toBe("true");
    } finally {
      localStorageMock.setItem = original;
    }
  });
});

describe("InstalledAgentCard setup links", () => {
  afterEach(() => {
    cleanup();
  });

  it("uses the base setup route when no agent record exists", () => {
    renderSetupCard();

    expect(screen.getByTestId("setup-profile-mock-agent").getAttribute("href")).toBe(
      "/settings/agents/mock-agent",
    );
  });

  it("uses create mode when a saved agent has no profiles", () => {
    renderSetupCard(makeSavedAgent("mock-agent", 0));

    expect(screen.getByTestId("setup-profile-mock-agent").getAttribute("href")).toBe(
      "/settings/agents/mock-agent?mode=create",
    );
  });
});
