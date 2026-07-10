import { afterEach, describe, expect, it, vi } from "vitest";
import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { listSentryProjects, searchSentryIssues } from "@/lib/api/domains/sentry-api";
import { useSentryInstances } from "@/hooks/domains/sentry/use-sentry-availability";
import type { SentryConfig } from "@/lib/types/sentry";

vi.mock("@/lib/api/domains/sentry-api", () => ({
  listSentryProjects: vi.fn(),
  searchSentryIssues: vi.fn(),
}));

vi.mock("@/hooks/domains/sentry/use-sentry-availability", () => ({
  useSentryInstances: vi.fn(),
}));

import { SentryIssueDialog } from "./sentry-issue-dialog";

const instance: SentryConfig = {
  id: "shared-instance",
  workspaceId: "workspace-1",
  name: "Production",
  authMethod: "auth_token",
  url: "https://sentry.example.com",
  hasSecret: true,
  lastOk: true,
  createdAt: "2026-01-01T00:00:00Z",
  updatedAt: "2026-01-01T00:00:00Z",
};

function mockAvailability() {
  vi.mocked(useSentryInstances).mockReturnValue({
    loading: false,
    instances: [instance],
    healthy: [instance],
    available: true,
    state: "single",
  });
}

afterEach(() => {
  cleanup();
  vi.clearAllMocks();
});

describe("SentryIssueDialog", () => {
  it("clears filters and results when its workspace changes", async () => {
    mockAvailability();
    vi.mocked(listSentryProjects).mockResolvedValue({
      projects: [{ id: "project-1", slug: "web", name: "Web", orgSlug: "acme" }],
    } as never);
    vi.mocked(searchSentryIssues).mockResolvedValue({
      issues: [
        {
          id: "issue-1",
          shortId: "WEB-1",
          title: "Stale workspace issue",
          permalink: "https://sentry.example.com/issues/1",
          projectSlug: "web",
          level: "error",
          status: "unresolved",
        },
      ],
      isLast: true,
    } as never);

    const view = render(
      <SentryIssueDialog open={true} onOpenChange={vi.fn()} workspaceId="workspace-1" />,
    );

    await waitFor(() => {
      expect((screen.getByLabelText("Organization") as HTMLInputElement).value).toBe("acme");
    });
    fireEvent.change(screen.getByLabelText("Query"), { target: { value: "is:unresolved" } });
    fireEvent.click(screen.getByRole("button", { name: "Search" }));
    await screen.findByText("Stale workspace issue");

    view.rerender(
      <SentryIssueDialog open={true} onOpenChange={vi.fn()} workspaceId="workspace-2" />,
    );

    expect((screen.getByLabelText("Query") as HTMLInputElement).value).toBe("");
    expect(screen.queryByText("Stale workspace issue")).toBeNull();
  });
});
