import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { cleanup, fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import { MarketplaceSourcesDialog } from "./marketplace-sources-dialog";
import type { MarketplaceSource } from "@/lib/types/plugins";

const marketplaceApi = vi.hoisted(() => ({
  addMarketplaceSource: vi.fn(),
  deleteMarketplaceSource: vi.fn(),
  updateMarketplaceSource: vi.fn(),
}));

vi.mock("@/lib/api/domains/marketplace-api", () => marketplaceApi);

afterEach(cleanup);

beforeEach(() => {
  vi.clearAllMocks();
  marketplaceApi.deleteMarketplaceSource.mockResolvedValue({ deleted: true });
  marketplaceApi.updateMarketplaceSource.mockResolvedValue({});
});

function source(overrides: Partial<MarketplaceSource> = {}): MarketplaceSource {
  return {
    id: "team",
    name: "Team Registry",
    url: "https://registry.example/team/index.json",
    enabled: true,
    builtin: false,
    healthy: true,
    ...overrides,
  };
}

function renderDialog(sources: MarketplaceSource[] = [source()]) {
  return render(
    <MarketplaceSourcesDialog open sources={sources} onOpenChange={vi.fn()} onChanged={vi.fn()} />,
  );
}

describe("MarketplaceSourcesDialog", () => {
  it("keeps source actions wired to their source while preserving the add form", async () => {
    renderDialog();

    expect(screen.getByTestId("marketplace-source-team")).toBeTruthy();
    expect(screen.getByTestId("marketplace-add-source-name")).toBeTruthy();
    expect(screen.getByTestId("marketplace-add-source-url")).toBeTruthy();

    fireEvent.click(screen.getByRole("switch"));
    await waitFor(() =>
      expect(marketplaceApi.updateMarketplaceSource).toHaveBeenCalledWith("team", {
        enabled: false,
      }),
    );

    fireEvent.click(screen.getByRole("button", { name: /Remove/i }));
    await waitFor(() =>
      expect(marketplaceApi.deleteMarketplaceSource).toHaveBeenCalledWith("team"),
    );
  });

  it("exposes one scroll body with the add form and touch-sized row actions outside it", () => {
    renderDialog([
      source({ id: "official", name: "Official", builtin: true }),
      source({ id: "team-2", name: "Team Two" }),
    ]);

    const dialog = screen.getByTestId("marketplace-sources-dialog");
    const sourceList = screen.getByTestId("marketplace-sources-list");
    const addForm = screen.getByTestId("marketplace-add-source-form");
    const remove = within(screen.getByTestId("marketplace-source-team-2")).getByRole("button", {
      name: /Remove/i,
    });

    expect(dialog.getAttribute("data-layout")).toBe("contained");
    expect(dialog.contains(sourceList)).toBe(true);
    expect(sourceList.contains(addForm)).toBe(false);
    expect(remove).toBeTruthy();
  });
});
