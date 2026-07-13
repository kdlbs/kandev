import { cleanup, render } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { useSentryIssueWatches } from "@/hooks/domains/sentry/use-sentry-issue-watches";
import { SentryIssueWatchersSection } from "./sentry-issue-watchers-section";

// The global active workspace intentionally differs from the routed workspace.
const h = vi.hoisted(() => ({ activeId: "ws-active" as string | null }));

vi.mock("@/components/state-provider", () => ({
  useAppStore: (sel: (s: unknown) => unknown) =>
    sel({ workspaces: { activeId: h.activeId, items: [] } }),
}));

vi.mock("@/hooks/domains/sentry/use-sentry-issue-watches", () => ({
  useSentryIssueWatches: vi.fn(() => ({
    items: [],
    loading: false,
    create: vi.fn(),
    update: vi.fn(),
    remove: vi.fn(),
    trigger: vi.fn(),
    previewReset: vi.fn(),
    reset: vi.fn(),
  })),
}));

vi.mock("@/hooks/domains/sentry/use-sentry-availability", () => ({
  useSentryInstances: vi.fn(() => ({
    instances: [],
    healthy: [],
    loading: false,
    available: false,
    state: "empty",
  })),
}));

vi.mock("@/components/toast-provider", () => ({ useToast: () => ({ toast: vi.fn() }) }));
vi.mock("./sentry-issue-watch-table", () => ({ SentryIssueWatchTable: () => null }));
vi.mock("./sentry-issue-watch-dialog", () => ({ SentryIssueWatchDialog: () => null }));
vi.mock("@/components/watches/reset-watch-dialog", () => ({
  ResetWatchDialog: () => null,
  useWatchResetController: () => ({
    resetting: null,
    setResetting: vi.fn(),
    onOpenChange: vi.fn(),
    previewLoader: vi.fn(),
    confirmReset: vi.fn(),
  }),
}));

afterEach(() => {
  cleanup();
  vi.clearAllMocks();
});

describe("SentryIssueWatchersSection scoping", () => {
  it("fetches watches only for its supplied workspace (never all workspaces)", () => {
    render(<SentryIssueWatchersSection workspaceId="ws-active" />);
    expect(vi.mocked(useSentryIssueWatches)).toHaveBeenCalledWith("ws-active");
    // never the unscoped `undefined` that would return foreign-workspace watches
    expect(vi.mocked(useSentryIssueWatches)).not.toHaveBeenCalledWith(undefined);
  });

  it("fetches watches for the routed workspace before the active workspace", () => {
    render(<SentryIssueWatchersSection workspaceId="ws-route" />);
    expect(vi.mocked(useSentryIssueWatches)).toHaveBeenCalledWith("ws-route");
    expect(vi.mocked(useSentryIssueWatches)).not.toHaveBeenCalledWith("ws-active");
  });
});
