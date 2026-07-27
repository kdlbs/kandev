import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { ToastProvider } from "@/components/toast-provider";
import type { PrepareGitHubAppImportResponse } from "@/lib/types/github";
import { GitHubAppImportForm } from "./github-app-import-form";

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

describe("GitHubAppImportForm", () => {
  it("clears imported secrets when setup starts over", async () => {
    const registrations = {
      mutating: false,
      prepareImport: vi.fn().mockResolvedValue(preparation),
      importRegistration: vi.fn(),
    };
    render(
      <ToastProvider>
        <GitHubAppImportForm
          workspaceId="workspace-1"
          registrations={registrations as never}
          onImported={vi.fn()}
        />
      </ToastProvider>,
    );

    fireEvent.change(screen.getByLabelText("Public Kandev URL"), {
      target: { value: "https://kandev.example" },
    });
    fireEvent.click(screen.getByRole("button", { name: "Generate setup instructions" }));
    const clientSecret = await screen.findByLabelText("Client secret");
    fireEvent.change(clientSecret, { target: { value: "client-secret" } });
    fireEvent.change(screen.getByLabelText("Webhook secret"), {
      target: { value: "webhook-secret" },
    });
    fireEvent.change(screen.getByLabelText("Private key (.pem)"), {
      target: { value: "private-key" },
    });

    fireEvent.click(screen.getByRole("button", { name: "Start over" }));
    fireEvent.click(screen.getByRole("button", { name: "Generate setup instructions" }));

    await waitFor(() => {
      expect((screen.getByLabelText("Client secret") as HTMLInputElement).value).toBe("");
      expect((screen.getByLabelText("Webhook secret") as HTMLInputElement).value).toBe("");
      expect((screen.getByLabelText("Private key (.pem)") as HTMLTextAreaElement).value).toBe("");
    });
  });
});
