import { describe, it, expect, vi, beforeEach } from "vitest";
import { renderHook } from "@testing-library/react";
import type { HealthIssue } from "@/lib/types/health";
import type { UpdatesResponse } from "@/lib/types/system";

type HealthMockReturn = {
  issues: HealthIssue[];
  healthy: boolean;
  loaded: boolean;
  loading: boolean;
};

type UpdatesMockReturn = {
  updates: UpdatesResponse | null;
};

const healthMock = vi.fn<[], HealthMockReturn>();
const updatesMock = vi.fn<[], UpdatesMockReturn>();

vi.mock("@/hooks/domains/settings/use-system-health", () => ({
  useSystemHealth: () => healthMock(),
}));

vi.mock("./use-updates", () => ({
  useUpdates: () => updatesMock(),
}));

import { useSystemBadgeCount } from "./use-system-badge-count";

function makeIssue(severity: HealthIssue["severity"], id: string = severity): HealthIssue {
  return {
    id,
    category: "test",
    title: `issue-${id}`,
    message: "",
    severity,
    fix_url: "",
    fix_label: "",
  };
}

function makeUpdates(updateAvailable: boolean): UpdatesResponse {
  return {
    current: "1.0.0",
    latest: updateAvailable ? "1.0.1" : "1.0.0",
    latest_url: "",
    latest_checked_at: "2026-05-18T00:00:00Z",
    update_available: updateAvailable,
  };
}

beforeEach(() => {
  healthMock.mockReset();
  updatesMock.mockReset();
});

describe("useSystemBadgeCount", () => {
  it("returns 0 when there are no non-info issues and no update is available", () => {
    healthMock.mockReturnValue({
      issues: [makeIssue("info")],
      healthy: true,
      loaded: true,
      loading: false,
    });
    updatesMock.mockReturnValue({ updates: makeUpdates(false) });
    const { result } = renderHook(() => useSystemBadgeCount());
    expect(result.current).toBe(0);
  });

  it("counts only non-info issues", () => {
    healthMock.mockReturnValue({
      issues: [makeIssue("info", "i1"), makeIssue("warning", "w1"), makeIssue("error", "e1")],
      healthy: false,
      loaded: true,
      loading: false,
    });
    updatesMock.mockReturnValue({ updates: null });
    const { result } = renderHook(() => useSystemBadgeCount());
    expect(result.current).toBe(2);
  });

  it("adds 1 when an update is available", () => {
    healthMock.mockReturnValue({
      issues: [],
      healthy: true,
      loaded: true,
      loading: false,
    });
    updatesMock.mockReturnValue({ updates: makeUpdates(true) });
    const { result } = renderHook(() => useSystemBadgeCount());
    expect(result.current).toBe(1);
  });

  it("sums non-info issues + update-available bump", () => {
    healthMock.mockReturnValue({
      issues: [makeIssue("warning", "w1"), makeIssue("error", "e1"), makeIssue("info", "i1")],
      healthy: false,
      loaded: true,
      loading: false,
    });
    updatesMock.mockReturnValue({ updates: makeUpdates(true) });
    const { result } = renderHook(() => useSystemBadgeCount());
    expect(result.current).toBe(3);
  });

  it("treats a missing updates payload as no update available", () => {
    healthMock.mockReturnValue({
      issues: [makeIssue("error", "e1")],
      healthy: false,
      loaded: true,
      loading: false,
    });
    updatesMock.mockReturnValue({ updates: null });
    const { result } = renderHook(() => useSystemBadgeCount());
    expect(result.current).toBe(1);
  });
});
