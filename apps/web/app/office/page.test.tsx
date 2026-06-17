import { describe, expect, it, vi, beforeEach } from "vitest";

const mocks = vi.hoisted(() => ({
  redirect: vi.fn((path: string) => {
    throw new Error(`redirect:${path}`);
  }),
  getOnboardingState: vi.fn(),
  getDashboard: vi.fn(),
  getActiveWorkspaceId: vi.fn(),
}));

vi.mock("@/lib/routing/server-navigation", () => ({
  redirect: mocks.redirect,
}));

vi.mock("@/lib/api/domains/office-api", () => ({
  getOnboardingState: mocks.getOnboardingState,
  getDashboard: mocks.getDashboard,
}));

vi.mock("./lib/get-active-workspace", () => ({
  getActiveWorkspaceId: mocks.getActiveWorkspaceId,
}));

vi.mock("./page-client", () => ({
  OfficePageClient: ({ initialDashboard }: { initialDashboard: unknown }) => (
    <div data-testid="office-page-client">{JSON.stringify(initialDashboard)}</div>
  ),
}));

import OfficePage from "./page";

describe("OfficePage", () => {
  beforeEach(() => {
    mocks.redirect.mockClear();
    mocks.getOnboardingState.mockReset();
    mocks.getDashboard.mockReset();
    mocks.getActiveWorkspaceId.mockReset();
  });

  it("redirects to setup-new when no office workspace exists", async () => {
    mocks.getOnboardingState.mockResolvedValue({ completed: true });
    mocks.getActiveWorkspaceId.mockResolvedValue(null);

    await expect(OfficePage({ searchParams: Promise.resolve({}) })).rejects.toThrow(
      "redirect:/office/setup?mode=new",
    );
    expect(mocks.redirect).toHaveBeenCalledWith("/office/setup?mode=new");
    expect(mocks.getDashboard).not.toHaveBeenCalled();
  });
});
