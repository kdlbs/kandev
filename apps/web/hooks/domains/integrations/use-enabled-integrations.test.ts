import { createElement } from "react";
import { act, cleanup, render, renderHook, screen } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { makeLocalStorageMock } from "@/hooks/local-storage-mock.test-helpers";
import {
  INTEGRATION_ENABLED_KEYS,
  type IntegrationSlug,
} from "@/lib/integrations/integration-enabled-keys";
import {
  IntegrationEnabledBadgeFor,
  IntegrationsEnabledProvider,
} from "@/components/app-sidebar/sections/settings/integration-enabled";
import { useEnabledIntegrations } from "./use-enabled-integrations";
import { useIntegrationEnabled } from "./use-integration-enabled";

const connections = vi.hoisted(() => new Map<string, boolean>());
const connected = (workspaceId?: string | null) => connections.get(workspaceId ?? "ws-1") ?? false;
vi.mock("@/hooks/domains/azure-devops/use-azure-devops-availability", () => ({
  useAzureDevOpsAvailable: (id: string) => connected(id),
}));
vi.mock("@/hooks/domains/github/use-github-status", () => ({
  useGitHubStatus: (id: string) => ({ status: { token_configured: connected(id) } }),
}));
vi.mock("@/hooks/domains/gitlab/use-task-mr", () => ({
  useGitLabAvailable: () => connected("ws-1"),
}));
vi.mock("@/hooks/domains/gitlab/use-gitlab-status", () => ({
  useGitLabStatus: (id: string) => ({ status: { authenticated: connected(id) } }),
}));
vi.mock("@/hooks/domains/jira/use-jira-availability", () => ({
  useJiraAuthed: (id: string) => connected(id),
}));
vi.mock("@/hooks/domains/linear/use-linear-availability", () => ({
  useLinearAuthed: (id: string) => connected(id),
}));
vi.mock("@/hooks/domains/sentry/use-sentry-availability", () => ({
  useSentryAvailable: (id: string) => connected(id),
}));

const storage = makeLocalStorageMock();
Object.defineProperty(window, "localStorage", { value: storage, configurable: true });
const slugs = Object.keys(INTEGRATION_ENABLED_KEYS) as IntegrationSlug[];

beforeEach(() => {
  storage.clear();
  connections.clear();
  connections.set("ws-1", true);
  connections.set("ws-2", true);
});
afterEach(cleanup);

// @covers AC-INTEGRATIONS-ENABLE-DISABLE-TOGGLE-001.3, AC-INTEGRATIONS-ENABLE-DISABLE-TOGGLE-001.9
describe("Settings integration enabled badges", () => {
  it.each(slugs)("does not badge connected %s when its saved workspace toggle is off", (slug) => {
    storage.setItem(`${INTEGRATION_ENABLED_KEYS[slug].storageKey}:ws-1`, "false");
    const { result } = renderHook(() => useEnabledIntegrations("ws-1"));
    expect(result.current.has(slug)).toBe(false);
    expect(result.current.size).toBe(slugs.length - 1);
  });

  it("updates both GitHub and GitLab after saving, keeping another workspace enabled", () => {
    const { result } = renderHook(() => ({
      first: useEnabledIntegrations("ws-1"),
      second: useEnabledIntegrations("ws-2"),
      github: useIntegrationEnabled(...toggleArgs("github"), "ws-1"),
      gitlab: useIntegrationEnabled(...toggleArgs("gitlab"), "ws-1"),
    }));
    expect(result.current.first.has("github")).toBe(true);
    expect(result.current.first.has("gitlab")).toBe(true);
    act(() => {
      result.current.github.setEnabled(false);
      result.current.gitlab.setEnabled(false);
    });
    expect(result.current.first.has("github")).toBe(false);
    expect(result.current.first.has("gitlab")).toBe(false);
    expect(result.current.second.has("github")).toBe(true);
    expect(result.current.second.has("gitlab")).toBe(true);
    act(() => {
      result.current.github.setEnabled(true);
      result.current.gitlab.setEnabled(true);
    });
    expect(result.current.first.has("github")).toBe(true);
    expect(result.current.first.has("gitlab")).toBe(true);
  });

  it("reacts to cross-tab storage changes", () => {
    const { result } = renderHook(() => useEnabledIntegrations("ws-1"));
    act(() => {
      storage.setItem(`${INTEGRATION_ENABLED_KEYS.gitlab.storageKey}:ws-1`, "false");
      window.dispatchEvent(new Event("storage"));
    });
    expect(result.current.has("gitlab")).toBe(false);
  });

  it("does not borrow the active workspace's GitLab connection", () => {
    connections.set("ws-2", false);
    const { result } = renderHook(() => useEnabledIntegrations("ws-2"));
    expect(result.current.size).toBe(0);
  });

  it("removes and restores the rendered badges when saved preferences change", () => {
    render(
      createElement(IntegrationsEnabledProvider, {
        workspaceId: "ws-1",
        children: createElement(
          "div",
          null,
          ...["github", "gitlab"].map((slug) =>
            createElement(
              "span",
              { key: slug },
              slug,
              createElement(IntegrationEnabledBadgeFor, { slug }),
            ),
          ),
        ),
      }),
    );
    expect(screen.getAllByText("Enabled")).toHaveLength(2);
    act(() => {
      for (const slug of ["github", "gitlab"] as const) {
        storage.setItem(`${INTEGRATION_ENABLED_KEYS[slug].storageKey}:ws-1`, "false");
        window.dispatchEvent(new Event(INTEGRATION_ENABLED_KEYS[slug].syncEvent));
      }
    });
    expect(screen.queryAllByText("Enabled")).toHaveLength(0);
    expect(screen.getByText("github")).toBeTruthy();
    expect(screen.getByText("gitlab")).toBeTruthy();
    act(() => {
      storage.clear();
      window.dispatchEvent(new Event("storage"));
    });
    expect(screen.getAllByText("Enabled")).toHaveLength(2);
  });
});

function toggleArgs(slug: IntegrationSlug): [string, string, string] {
  const keys = INTEGRATION_ENABLED_KEYS[slug];
  return [keys.storageKey, keys.legacyKeyPrefix, keys.syncEvent];
}
