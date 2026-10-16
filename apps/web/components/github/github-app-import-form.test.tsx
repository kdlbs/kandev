const PUBLIC_KANDEV_URL = "https://kandev.example";
const CLIENT_SECRET_LABEL = "Client secret";
const WEBHOOK_SECRET_LABEL = "Webhook secret";
const PRIVATE_KEY_LABEL = "Private key (.pem)";
import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { ToastProvider } from "@/components/toast-provider";
import type { PrepareGitHubAppImportResponse } from "@/lib/types/github";
import { GitHubAppImportForm } from "./github-app-import-form";

afterEach(() => cleanup());

const preparation: PrepareGitHubAppImportResponse = {
  registration_id: "registration-1",
  public_base_url: PUBLIC_KANDEV_URL,
  manifest_callback_url: "https://kandev.example/manifest/callback",
  install_callback_url: "https://kandev.example/install/callback",
  personal_callback_url: "https://kandev.example/personal/callback",
  webhook_url: "https://kandev.example/webhook",
  setup_url: "",
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
      target: { value: PUBLIC_KANDEV_URL },
    });
    fireEvent.click(screen.getByRole("button", { name: "Generate setup instructions" }));
    const clientSecret = await screen.findByLabelText(CLIENT_SECRET_LABEL);
    fireEvent.change(clientSecret, { target: { value: "client-secret" } });
    fireEvent.change(screen.getByLabelText(WEBHOOK_SECRET_LABEL), {
      target: { value: "webhook-secret" },
    });
    fireEvent.change(screen.getByLabelText(PRIVATE_KEY_LABEL), {
      target: { value: "private-key" },
    });

    fireEvent.click(screen.getByRole("button", { name: "Start over" }));
    fireEvent.click(screen.getByRole("button", { name: "Generate setup instructions" }));

    await waitFor(() => {
      expect((screen.getByLabelText(CLIENT_SECRET_LABEL) as HTMLInputElement).value).toBe("");
      expect((screen.getByLabelText(WEBHOOK_SECRET_LABEL) as HTMLInputElement).value).toBe("");
      expect((screen.getByLabelText(PRIVATE_KEY_LABEL) as HTMLTextAreaElement).value).toBe("");
    });
  });
});

it("submits App secrets on the selected host and clears them after failed verification", async () => {
  const registrations = {
    mutating: false,
    prepareImport: vi.fn().mockResolvedValue(preparation),
    importRegistration: vi.fn().mockRejectedValue(new Error("Provider denied import")),
  };
  render(
    <ToastProvider>
      <GitHubAppImportForm
        workspaceId="workspace-1"
        githubHost="git.company.example"
        registrations={registrations as never}
        onImported={vi.fn()}
      />
    </ToastProvider>,
  );
  fireEvent.change(screen.getByLabelText("Public Kandev URL"), {
    target: { value: PUBLIC_KANDEV_URL },
  });
  fireEvent.click(screen.getByRole("button", { name: "Generate setup instructions" }));
  await screen.findByLabelText(CLIENT_SECRET_LABEL);
  for (const [label, value] of [
    ["Name in Kandev", "Enterprise App"],
    ["GitHub App slug", "enterprise-app"],
    ["GitHub owner login", "acme"],
    ["App ID", "42"],
    ["Client ID", "client-id"],
    [CLIENT_SECRET_LABEL, "client-secret"],
    [WEBHOOK_SECRET_LABEL, "webhook-secret"],
    [PRIVATE_KEY_LABEL, "private-key"],
  ])
    fireEvent.change(screen.getByLabelText(label), { target: { value } });
  fireEvent.click(screen.getByRole("button", { name: "Verify and import App" }));
  await waitFor(() =>
    expect(registrations.importRegistration).toHaveBeenCalledWith(
      expect.objectContaining({
        github_host: "git.company.example",
        client_secret: "client-secret",
      }),
    ),
  );
  await waitFor(() =>
    expect((screen.getByLabelText(CLIENT_SECRET_LABEL) as HTMLInputElement).value).toBe(""),
  );
  expect((screen.getByLabelText(WEBHOOK_SECRET_LABEL) as HTMLInputElement).value).toBe("");
  expect((screen.getByLabelText(PRIVATE_KEY_LABEL) as HTMLTextAreaElement).value).toBe("");
  expect((screen.getByLabelText("Name in Kandev") as HTMLInputElement).value).toBe(
    "Enterprise App",
  );
  expect(registrations.prepareImport).toHaveBeenCalledWith({
    public_base_url: PUBLIC_KANDEV_URL,
    github_host: "git.company.example",
  });
});
