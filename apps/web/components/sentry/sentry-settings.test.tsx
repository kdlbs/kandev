import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { ReactElement } from "react";
import { act, cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { TooltipProvider } from "@kandev/ui/tooltip";
import { listSentryInstances } from "@/lib/api/domains/sentry-api";
import { qk } from "@/lib/query/keys";
import type { SentryConfig } from "@/lib/types/sentry";
import { SettingsSaveProvider } from "@/components/settings/settings-save-provider";

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
  INTEGRATION_STATUS_REFRESH_MS: 10_000,
}));

vi.mock("@/hooks/domains/workspace/use-workspaces", () => ({
  useWorkspaces: () => ({
    items: mocks.workspaces,
    activeId: mocks.activeWorkspaceId,
    activeWorkspace: mocks.workspaces.find((w) => w.id === mocks.activeWorkspaceId) ?? null,
  }),
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

const WORKSPACE_ID = "workspace-1";

const instance: SentryConfig = {
  id: "instance-1",
  workspaceId: WORKSPACE_ID,
  name: "Production",
  authMethod: "auth_token",
  url: "https://sentry.example.com",
  hasSecret: true,
  lastOk: true,
  createdAt: "2026-01-01T00:00:00Z",
  updatedAt: "2026-01-01T00:00:00Z",
};

function createQueryClient() {
  return new QueryClient({
    defaultOptions: {
      queries: { retry: false },
      mutations: { retry: false },
    },
  });
}

function renderWithProviders(ui: ReactElement, queryClient = createQueryClient()) {
  return {
    queryClient,
    ...render(
      <QueryClientProvider client={queryClient}>
        <SettingsSaveProvider>
          <TooltipProvider>{ui}</TooltipProvider>
        </SettingsSaveProvider>
      </QueryClientProvider>,
    ),
  };
}

function renderWithHydratedInstances(ui: ReactElement, instances: SentryConfig[]) {
  const queryClient = createQueryClient();
  queryClient.setQueryDefaults(qk.integrations.sentry.instances(WORKSPACE_ID), {
    staleTime: Infinity,
  });
  queryClient.setQueryData(qk.integrations.sentry.instances(WORKSPACE_ID), instances);
  vi.mocked(listSentryInstances).mockResolvedValue(instances);
  return renderWithProviders(ui, queryClient);
}

beforeEach(() => {
  mocks.activeWorkspaceId = "ws-active";
});

afterEach(() => {
  cleanup();
  vi.clearAllMocks();
});

describe("SentryConnectionSection", () => {
  it("restores the add path when a polling refresh removes the instance being edited", async () => {
    const { queryClient } = renderWithHydratedInstances(
      <SentryConnectionSection workspaceId={WORKSPACE_ID} />,
      [instance],
    );

    expect(screen.getByTestId("sentry-instance-edit-button")).toBeTruthy();
    fireEvent.click(screen.getByTestId("sentry-instance-edit-button"));
    expect(screen.getByTestId("sentry-edit-form")).toBeTruthy();

    act(() => {
      queryClient.setQueryData(qk.integrations.sentry.instances(WORKSPACE_ID), []);
    });

    await waitFor(() => expect(screen.getByRole("button", { name: "Add instance" })).toBeTruthy());
  });

  it("keeps initial load failures visible but silences recurring poll failures", async () => {
    vi.mocked(listSentryInstances).mockRejectedValue(new Error("offline"));

    renderWithProviders(<SentryConnectionSection workspaceId={WORKSPACE_ID} />);

    await waitFor(() => expect(mocks.toast).toHaveBeenCalledTimes(1));

    expect(listSentryInstances).toHaveBeenCalledTimes(1);
    expect(mocks.toast).toHaveBeenCalledTimes(1);
  });
});

describe("SentryIntegrationPage workspace scope", () => {
  it("passes the route workspace to watchers before the global active workspace", () => {
    vi.mocked(listSentryInstances).mockResolvedValue([]);

    renderWithProviders(<SentryIntegrationPage workspaceId="ws-route" />);

    expect(screen.getByTestId("sentry-watchers-workspace").textContent).toBe("ws-route");
  });

  it("uses the global active workspace when no route workspace is supplied", () => {
    vi.mocked(listSentryInstances).mockResolvedValue([]);

    renderWithProviders(<SentryIntegrationPage />);

    expect(screen.getByTestId("sentry-watchers-workspace").textContent).toBe("ws-active");
  });
});
