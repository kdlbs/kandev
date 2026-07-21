import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it } from "vitest";
import type { PrepareGitHubAppImportResponse } from "@/lib/types/github";
import { GitHubAppImportGuide } from "./github-app-import-guide";

afterEach(() => cleanup());

const preparation: PrepareGitHubAppImportResponse = {
  registration_id: "registration-1",
  public_base_url: "https://kandev.example",
  manifest_callback_url: "https://kandev.example/manifest/callback",
  install_callback_url: "https://kandev.example/install/callback",
  personal_callback_url: "https://kandev.example/personal/callback",
  webhook_url: "https://kandev.example/webhook",
  setup_url: "https://kandev.example/install/callback",
  permissions: { contents: "write" },
  events: ["installation"],
  expires_at: "2026-07-22T00:00:00Z",
};

describe("GitHubAppImportGuide", () => {
  it("shows the exact existing-App settings and operational requirements", () => {
    render(
      <GitHubAppImportGuide
        preparation={preparation}
        settingsUrl="https://github.com/organizations/acme/settings/apps/work-app"
      />,
    );
    expect(screen.getByText("Homepage URL")).toBeTruthy();
    expect(screen.getByText("User authorization callback URL")).toBeTruthy();
    expect(screen.getByText("Setup URL")).toBeTruthy();
    expect(screen.getByText("Webhook URL")).toBeTruthy();
    expect(screen.getByText(/application\/json/)).toBeTruthy();
    expect(screen.getByText(/SSL verification enabled/)).toBeTruthy();
    expect(screen.queryByText("https://kandev.example/manifest/callback")).toBeNull();
  });

  it("does not link to generic settings before the App identity is known", () => {
    render(<GitHubAppImportGuide preparation={preparation} />);
    const button = screen.getByRole("button", { name: "Open GitHub App settings" });
    expect((button as HTMLButtonElement).disabled).toBe(true);
  });
});
