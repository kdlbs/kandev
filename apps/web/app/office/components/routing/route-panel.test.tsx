import { afterEach, describe, expect, it, vi } from "vitest";
import { cleanup, render, screen } from "@testing-library/react";
import type { RouteAttempt } from "@/lib/state/slices/office/types";

let attempts: RouteAttempt[] = [];
vi.mock("@/hooks/domains/office/use-run-attempts", () => ({
  useRunAttempts: () => ({ attempts, isLoading: false, error: null, refresh: vi.fn() }),
}));

import { RoutePanel } from "./route-panel";

describe("RoutePanel", () => {
  afterEach(() => {
    cleanup();
    attempts = [];
  });

  it("labels an author-provider skip as the author exclusion", () => {
    attempts = [
      {
        seq: 1,
        provider_id: "claude-acp",
        model: "sonnet",
        tier: "balanced",
        outcome: "skipped_author_provider",
        started_at: "2026-11-17T10:00:00Z",
      },
    ];
    render(<RoutePanel runId="run-1" />);
    expect(screen.getByText("skipped: wrote this task")).toBeTruthy();
  });
});
