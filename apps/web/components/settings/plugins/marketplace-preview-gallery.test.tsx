import { afterEach, describe, expect, it, vi } from "vitest";
import { cleanup, fireEvent, render, screen } from "@testing-library/react";

vi.mock("react-i18next", () => ({
  useTranslation: () => ({
    t: (key: string, values?: Record<string, unknown>) => {
      if (key === "plugins:previewPosition") return `${values?.current}/${values?.total}`;
      return key;
    },
  }),
}));

import { MarketplacePreviewGallery } from "./marketplace-preview-gallery";

afterEach(() => cleanup());

describe("MarketplacePreviewGallery", () => {
  it("keeps ordered previews navigable and exposes the selected alt text", () => {
    render(
      <MarketplacePreviewGallery
        previews={[
          { url: "https://cdn.test/one.png", alt: "First screen" },
          { url: "https://cdn.test/two.png", alt: "Second screen" },
        ]}
      />,
    );

    expect(screen.getByAltText("First screen")).toBeTruthy();
    expect(screen.getByText("1/2")).toBeTruthy();
    fireEvent.click(screen.getByRole("button", { name: "plugins:nextPreview" }));
    expect(screen.getByAltText("Second screen")).toBeTruthy();
    expect(screen.getByText("2/2")).toBeTruthy();
  });

  it("shows a retry state when the cover image fails", () => {
    render(
      <MarketplacePreviewGallery
        previews={[{ url: "https://cdn.test/bad.png", alt: "Preview" }]}
      />,
    );
    fireEvent.error(screen.getByAltText("Preview"));
    expect(screen.getByText("plugins:previewImageFailed")).toBeTruthy();
    expect(screen.getByRole("button", { name: "plugins:retry" })).toBeTruthy();
  });
});
