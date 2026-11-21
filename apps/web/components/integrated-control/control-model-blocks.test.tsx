import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { ApiError } from "@/lib/api/client";
import { ControlModelBlocks } from "./control-model-blocks";

const fixture = vi.hoisted(() => ({
  data: {
    workspaces: [
      { id: "w1", name: "One" },
      { id: "w2", name: "Two" },
    ],
  },
  readHealth: vi.fn(),
}));
vi.mock("react-i18next", () => ({ useTranslation: () => ({ t: (key: string) => key }) }));
vi.mock("@/components/state-provider", () => ({ useAppStore: () => "owner-one" }));
vi.mock("./integrated-control-provider", () => ({
  useIntegratedControl: () => ({ data: fixture.data }),
}));
vi.mock("@/hooks/domains/coordinator/use-control-overview", () => ({
  useControlOverview: () => ({ status: "unsupported" }),
}));
vi.mock("@/lib/api/domains/office-routing-api", () => ({ getProviderHealth: fixture.readHealth }));

const warning = {
  workspace_id: "w1",
  provider_id: "provider-one",
  scope: "model",
  scope_value: "recorded-model",
  state: "user_action_required",
  error_code: "account_blocked",
  retry_at: "2000-01-01T00:00:00Z",
  backoff_step: 1,
};
afterEach(cleanup);
beforeEach(() => {
  fixture.readHealth.mockReset();
});

describe("native workspace provider health", () => {
  it("shows in-scope recorded health with an elapsed retry that does not imply recovery", async () => {
    fixture.readHealth.mockResolvedValue({ health: [warning] });
    render(<ControlModelBlocks workspaceId="w1" />);
    const row = await screen.findByTestId("control-provider-health-row");
    expect(row.textContent).toContain("recorded-model");
    expect(row.textContent).toContain("office:needsAction");
    expect(row.textContent).toContain("account_blocked");
    expect(row.textContent).toContain("sidebar:controlRestore.retryElapsed");
    expect(row.textContent).not.toContain("sidebar:controlRestore.cleared");
    expect(fixture.readHealth.mock.calls.map(([id]) => id)).toEqual(["w1"]);
  });

  it("keeps a successful workspace visible alongside a denied workspace", async () => {
    fixture.readHealth.mockImplementation(async (id: string) => {
      if (id === "w2") throw new ApiError("denied", 403, null);
      return { health: [warning] };
    });
    render(<ControlModelBlocks />);
    expect(await screen.findByTestId("control-provider-health-row")).toBeTruthy();
    expect(await screen.findByText("sidebar:integratedControl.readDenied")).toBeTruthy();
    expect(screen.queryByText("office:overviewNoBlockedModels")).toBeNull();
  });

  it.each([
    [403, "sidebar:integratedControl.readDenied"],
    [404, "sidebar:controlRestore.contractUnavailable"],
    [500, "sidebar:integratedControl.loadError"],
  ])("does not present HTTP %s as an empty healthy list", async (status, key) => {
    fixture.readHealth.mockRejectedValue(new ApiError("read failed", status, null));
    render(<ControlModelBlocks workspaceId="w1" />);
    expect(await screen.findByText(key)).toBeTruthy();
    expect(screen.queryByText("sidebar:controlRestore.noProviderHealth")).toBeNull();
  });

  it("reports known empty provider health without claiming complete model/account coverage", async () => {
    fixture.readHealth.mockResolvedValue({ health: [] });
    render(<ControlModelBlocks workspaceId="w1" />);
    expect(await screen.findByText("sidebar:controlRestore.noProviderHealth")).toBeTruthy();
  });

  it("rejects an incompatible response and never reveals a foreign workspace row", async () => {
    fixture.readHealth.mockResolvedValue({ health: [{ ...warning, workspace_id: "foreign" }] });
    render(<ControlModelBlocks workspaceId="w1" />);
    expect(await screen.findByText("sidebar:controlRestore.contractUnavailable")).toBeTruthy();
    expect(screen.queryByText("recorded-model")).toBeNull();
    expect(screen.queryByText("sidebar:controlRestore.noProviderHealth")).toBeNull();
  });
});
