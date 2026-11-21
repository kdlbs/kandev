import { act, cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { clearNavigationBlockerForTests } from "@/lib/routing/navigation-guard";
import type {
  ProviderAccountLimit,
  ProviderLimit,
  UpdateProviderLimitRequest,
} from "@/lib/api/domains/provider-limits-api";
import { SettingsSaveProvider, useSettingsSaveCoordinator } from "./settings-save-provider";
import { ProviderLimitsCard } from "./provider-limits-card";

const api = vi.hoisted(() => ({
  listProviderLimits: vi.fn(),
  updateProviderLimit: vi.fn(),
}));

vi.mock("@/lib/api/domains/provider-limits-api", () => api);
vi.mock("@/hooks/domains/features/use-feature", () => ({ useFeature: () => true }));

const ACCOUNT: ProviderAccountLimit = {
  account_id: "acct-1",
  account_kind: "api_key",
  profiles: [],
  overrides: { tiers: {} },
  tiers: {} as ProviderAccountLimit["tiers"],
};

function providerLimit(): ProviderLimit {
  return { provider: "acme", profile_count: 1, model_scoped: false, accounts: [ACCOUNT] };
}

function DirtyObserver() {
  const coordinator = useSettingsSaveCoordinator();
  return <output data-testid="observed-dirty">{String(coordinator.hasDirty)}</output>;
}

async function mountCard() {
  render(
    <SettingsSaveProvider>
      <ProviderLimitsCard />
      <DirtyObserver />
    </SettingsSaveProvider>,
  );
  await screen.findByTestId("provider-limit-acme");
}

function expectDirty(value: boolean) {
  expect(screen.getByTestId("observed-dirty").textContent).toBe(String(value));
}

function input(id: string) {
  return document.getElementById(id) as HTMLInputElement;
}

beforeEach(() => {
  api.listProviderLimits.mockResolvedValue({ providers: [providerLimit()] });
  api.updateProviderLimit.mockImplementation(
    async (provider: string, request: UpdateProviderLimitRequest) => ({
      ...providerLimit(),
      provider,
      accounts: request.account_id
        ? [{ ...ACCOUNT, overrides: request.account_limits ?? { tiers: {} } }]
        : [ACCOUNT],
    }),
  );
});

afterEach(() => {
  cleanup();
  clearNavigationBlockerForTests();
  vi.clearAllMocks();
});

describe("provider limits shared save", () => {
  it("has no card-local save or reset controls", async () => {
    await mountCard();
    expect(screen.queryByTestId("account-save-acct-1")).toBeNull();
    expect(screen.queryByRole("button", { name: "Save" })).toBeNull();
    expect(screen.queryByRole("button", { name: "Clear" })).toBeNull();
    expect(screen.queryByRole("button", { name: "Clear account overrides" })).toBeNull();
  });

  it("saves an account tier edit through the shared save", async () => {
    await mountCard();
    expectDirty(false);
    fireEvent.change(input("account-acct-1-paid-requests_per_minute"), {
      target: { value: "7" },
    });
    expectDirty(true);
    expect(api.updateProviderLimit).not.toHaveBeenCalled();

    await act(async () => {
      fireEvent.click(screen.getByRole("button", { name: "Save changes" }));
    });
    await waitFor(() => expectDirty(false));
    expect(api.updateProviderLimit).toHaveBeenCalledTimes(1);
    expect(api.updateProviderLimit).toHaveBeenCalledWith("acme", {
      account_id: "acct-1",
      account_limits: { tiers: { paid: { requests_per_minute: 7 } } },
      monthly_reset_at: null,
      monthly_reset_timezone: "",
      block_until: null,
    });
    expect(input("account-acct-1-paid-requests_per_minute").value).toBe("7");
  });

  it("keeps a failed account save dirty", async () => {
    api.updateProviderLimit.mockRejectedValueOnce(new Error("rejected"));
    await mountCard();
    fireEvent.change(input("account-acct-1-paid-requests_per_minute"), {
      target: { value: "7" },
    });
    await act(async () => {
      fireEvent.click(screen.getByRole("button", { name: "Save changes" }));
    });
    await waitFor(() => expect(api.updateProviderLimit).toHaveBeenCalledTimes(1));
    expectDirty(true);
  });

  it("saves the provider block through the shared save and discards with the shared reset", async () => {
    await mountCard();
    fireEvent.change(input("provider-limit-block-acme"), { target: { value: "2026-12-01T10:00" } });
    expectDirty(true);

    await act(async () => {
      fireEvent.click(screen.getByRole("button", { name: "Save changes" }));
    });
    await waitFor(() => expectDirty(false));
    expect(api.updateProviderLimit).toHaveBeenCalledTimes(1);
    const [provider, request] = api.updateProviderLimit.mock.calls[0];
    expect(provider).toBe("acme");
    expect(request.account_id).toBeUndefined();
    expect(request.monthly_reset_at).toBeNull();
    expect(request.block_until).toBe(new Date("2026-12-01T10:00").toISOString());

    fireEvent.change(input("provider-limit-block-acme"), { target: { value: "" } });
    expectDirty(true);
    fireEvent.click(screen.getByRole("button", { name: "Reset" }));
    await waitFor(() => expectDirty(false));
    expect(input("provider-limit-block-acme").value).toBe("2026-12-01T10:00");
    expect(api.updateProviderLimit).toHaveBeenCalledTimes(1);
  });
});
