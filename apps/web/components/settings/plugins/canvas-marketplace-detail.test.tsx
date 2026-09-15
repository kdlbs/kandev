import { afterEach, describe, expect, it, vi } from "vitest";
import { cleanup, render, screen } from "@testing-library/react";
import type { MarketplaceEntry } from "@/lib/types/plugins";

vi.mock("react-i18next", () => ({ useTranslation: () => ({ t: (key: string) => key }) }));
vi.mock("./marketplace-preview-gallery", () => ({
  MarketplacePreviewGallery: () => <div data-testid="preview-gallery" />,
}));
vi.mock("./plugin-repo-link", () => ({
  PluginRepoLink: () => <a href="https://example.test/repo">plugins:repo</a>,
}));

import { CanvasMarketplaceDetail } from "./canvas-marketplace-detail";

afterEach(() => cleanup());

const entry: MarketplaceEntry = {
  id: "canvas-one",
  kind: "canvas",
  name: "Canvas One",
  description: "A reviewed canvas",
  author: "author",
  categories: ["planning"],
  icon_url: "",
  repo_url: "https://example.test/repo",
  version: "1.0.0",
  min_kandev_version: "0.94.0",
  license: "MIT",
  package_url: "https://example.test/canvas.tar.gz",
  package_sha256: "digest",
  previews: [{ url: "https://example.test/cover.png", alt: "Cover" }],
  permissions: {
    reads: ["tasks"],
    writes: [],
    events: ["task.updated"],
    shared_state: true,
    external_origins: [],
  },
  stars: 2,
  updated_at: "2026-09-11T00:00:00Z",
  install_state: "available",
  source_id: "official",
  source_name: "Official",
};

describe("CanvasMarketplaceDetail", () => {
  it("shows the shared gallery, permission groups, and review action", () => {
    render(<CanvasMarketplaceDetail entry={entry} onBack={vi.fn()} onInstall={vi.fn()} />);

    expect(screen.getByTestId("preview-gallery")).toBeTruthy();
    expect(screen.getByText("A reviewed canvas")).toBeTruthy();
    expect(screen.getByText("tasks")).toBeTruthy();
    expect(screen.getByText("task.updated")).toBeTruthy();
    expect(screen.getByRole("button", { name: "plugins:reviewAndInstall" })).toBeTruthy();
  });
});
