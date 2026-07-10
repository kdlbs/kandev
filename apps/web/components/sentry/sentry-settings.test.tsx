import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { act, cleanup, fireEvent, render, screen } from "@testing-library/react";
import { TooltipProvider } from "@kandev/ui/tooltip";
import { listSentryInstances } from "@/lib/api/domains/sentry-api";
import type { SentryConfig } from "@/lib/types/sentry";

const mocks = vi.hoisted(() => ({
  activeWorkspaceId: "ws-active",
  setEnabled: vi.fn(),
  toast: vi.fn(),
  workspaces: [
    { id: "ws-active", name: "Active" },
    { id: "ws-route", name: "Route" },
  ],
}));

vi.mock("@/components/toast-provider", () => ({
  useToast: () => ({ toast: mocks.toast }),
}));

vi.mock("@/hooks/domains/sentry/use-sentry-enabled", () => ({
  useSentryEnabled: () => ({ enabled: true, setEnabled: mocks.setEnabled }),
}));

vi.mock("@/hooks/domains/integrations/use-integration-availability", () => ({
  INTEGRATION_STATUS_REFRESH_MS: 1,
}));

vi.mock("@kandev/ui/switch", () => ({
  Switch: ({ onCheckedChange: _onCheckedChange, ...props }: Record<string, unknown>) => (
    <button {...props} />
  ),
}));

vi.mock("@/components/state-provider", () => ({
  useAppStore: (
    selector: (state: {
      workspaces: { activeId: string; items: { id: string; name: string }[] };
    }) => unknown,
  ) =>
    selector({
      workspaces: {
        activeId: mocks.activeWorkspaceId,
        items: mocks.workspaces,
      },
    }),
}));

vi.mock("./sentry-issue-watchers-section", () => ({
  SentryIssueWatchersSection: ({ workspaceId }: { workspaceId: string }) => (
    <div data-testid="sentry-watchers-workspace">{workspaceId}</div>
  ),
}));

vi.mock("@/lib/api/domains/sentry-api", () => ({
  createSentryInstance: vi.fn(),
  deleteSentryInstance: vi.fn(),
  listSentryInstances: vi.fn(),
  sentryErrorCode: vi.fn(),
  sentryInUseWatchCount: vi.fn(),
  testSentryConnection: vi.fn(),
  testSentryInstance: vi.fn(),
  updateSentryInstance: vi.fn(),
  SENTRY_ERROR_CODES: { nameTaken: "SENTRY_INSTANCE_NAME_TAKEN" },
}));

import { SentryConnectionSection, SentryIntegrationPage } from "./sentry-settings";

const instance: SentryConfig = {
  id: "instance-1",
  workspaceId: "workspace-1",
  name: "Production",
  authMethod: "auth_token",
  url: "https://sentry.example.com",
  hasSecret: true,
  lastOk: true,
  createdAt: "2026-01-01T00:00:00Z",
  updatedAt: "2026-01-01T00:00:00Z",
};

beforeEach(() => {
  vi.useFakeTimers();
  mocks.activeWorkspaceId = "ws-active";
});

afterEach(() => {
  cleanup();
  vi.useRealTimers();
  vi.clearAllMocks();
});

describe("SentryConnectionSection", () => {
  it("restores the add path when a polling refresh removes the instance being edited", async () => {
    vi.mocked(listSentryInstances).mockResolvedValueOnce([instance]).mockResolvedValueOnce([]);

    render(
      <TooltipProvider>
        <SentryConnectionSection workspaceId="workspace-1" />
      </TooltipProvider>,
    );

    await act(async () => {
      await Promise.resolve();
    });
    expect(screen.getByTestId("sentry-instance-edit-button")).toBeTruthy();
    fireEvent.click(screen.getByTestId("sentry-instance-edit-button"));
    expect(screen.getByTestId("sentry-edit-form")).toBeTruthy();

    await act(async () => {
      await vi.advanceTimersByTimeAsync(1);
    });

    expect(screen.getByRole("button", { name: "Add instance" })).toBeTruthy();
  });

  it("keeps initial load failures visible but silences recurring poll failures", async () => {
    vi.mocked(listSentryInstances).mockRejectedValue(new Error("offline"));

    render(
      <TooltipProvider>
        <SentryConnectionSection workspaceId="workspace-1" />
      </TooltipProvider>,
    );

    await act(async () => {
      await Promise.resolve();
    });
    expect(mocks.toast).toHaveBeenCalledTimes(1);

    await act(async () => {
      await vi.advanceTimersByTimeAsync(1);
    });

    expect(mocks.toast).toHaveBeenCalledTimes(1);
  });
});

describe("SentryIntegrationPage workspace scope", () => {
  it("passes the route workspace to watchers before the global active workspace", () => {
    vi.mocked(listSentryInstances).mockResolvedValue([]);

    render(
      <TooltipProvider>
        <SentryIntegrationPage workspaceId="ws-route" />
      </TooltipProvider>,
    );

    expect(screen.getByTestId("sentry-watchers-workspace").textContent).toBe("ws-route");
  });

  it("uses the global active workspace when no route workspace is supplied", () => {
    vi.mocked(listSentryInstances).mockResolvedValue([]);

    render(
      <TooltipProvider>
        <SentryIntegrationPage />
      </TooltipProvider>,
    );

    expect(screen.getByTestId("sentry-watchers-workspace").textContent).toBe("ws-active");
  });
});
