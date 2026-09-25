import { describe, expect, it } from "vitest";
import { previewSourceLabel, sanitizePreviewPageRoute } from "./preview-feedback-source";

describe("preview feedback source identity", () => {
  it("keeps browser source labels free of credentials and route data", () => {
    expect(
      previewSourceLabel(
        "https://user:password@example.test:8443/account?access_token=secret#fragment",
      ),
    ).toBe("https://example.test:8443");
  });

  it("keeps safe route parameters while dropping secrets and fragments", () => {
    expect(
      sanitizePreviewPageRoute(
        "/checkout?step=shipping&access_token=secret&redirect_uri=%2Faccount#payment",
      ),
    ).toBe("/checkout?step=shipping&redirect_uri=%2Faccount");
  });
});
