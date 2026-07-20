import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import type {
  DeploymentGitHubAppRegistration,
  DeploymentGitHubAppStatus,
} from "@/lib/types/github";

const mocks = vi.hoisted(() => ({
  search: "",
  toast: vi.fn(),
  useRegistration: vi.fn(),
}));

vi.mock("@/hooks/domains/github/use-deployment-app-registration", () => ({
  useDeploymentAppRegistration: mocks.useRegistration,
}));

vi.mock("@/components/toast-provider", () => ({
  useToast: () => ({ toast: mocks.toast }),
}));

vi.mock("@/lib/routing/client-router", () => ({
  useSearchParams: () => new URLSearchParams(mocks.search),
}));

import { GitHubAppSettings } from "./github-app-settings";

const registration: DeploymentGitHubAppRegistration = {
  github_host: "github.com",
  app_id: 42,
  client_id: "client-id",
  slug: "kandev-acme",
  owner_login: "acme",
  owner_type: "Organization",
  public_base_url: "https://kandev.example",
  credential_generation: 1,
  webhook_status: "verified",
  last_webhook_at: "2026-07-20T20:00:00Z",
  created_at: "2026-07-20T19:00:00Z",
  updated_at: "2026-07-20T20:00:00Z",
};

function setStatus(status: DeploymentGitHubAppStatus, error: string | null = null) {
  mocks.useRegistration.mockReturnValue({
    status,
    loading: false,
    mutating: false,
    error,
    reload: vi.fn().mockResolvedValue(true),
    start: vi.fn(),
    remove: vi.fn(),
  });
}

beforeEach(() => {
  mocks.search = "";
  mocks.toast.mockReset();
  mocks.useRegistration.mockReset();
});

afterEach(() => cleanup());

describe("GitHubAppSettings", () => {
  it("renders managed registration and webhook health without exposing setup controls", () => {
    setStatus({
      source: "managed",
      state: "ready",
      ready: true,
      read_only: false,
      registration,
    });

    render(<GitHubAppSettings />);

    expect(screen.getByTestId("github-app-status").getAttribute("data-source")).toBe("managed");
    expect(screen.getByText("Webhook verified")).toBeTruthy();
    expect(screen.getByTestId("github-app-remove-button")).toBeTruthy();
    expect(screen.queryByTestId("github-app-setup-form")).toBeNull();
  });

  it("renders environment registrations as read-only", () => {
    setStatus({
      source: "environment",
      state: "ready",
      ready: true,
      read_only: true,
      registration: { ...registration, webhook_status: "unverified" },
    });

    render(<GitHubAppSettings />);

    expect(screen.getByTestId("github-app-environment-status").textContent).toContain(
      "Environment configuration has priority",
    );
    expect(screen.queryByTestId("github-app-remove-button")).toBeNull();
  });

  it("suppresses stale status controls when the current load failed", () => {
    setStatus(
      {
        source: "managed",
        state: "ready",
        ready: true,
        read_only: false,
        registration,
      },
      "refresh unavailable",
    );

    render(<GitHubAppSettings />);

    expect(screen.getByTestId("github-app-status-error").textContent).toContain(
      "refresh unavailable",
    );
    expect(screen.queryByTestId("github-app-remove-button")).toBeNull();
  });

  it("shows unknown callback results and opens required permissions", async () => {
    mocks.search = "github_app_result=future_error";
    setStatus({
      source: "none",
      state: "unconfigured",
      ready: false,
      read_only: false,
    });

    render(<GitHubAppSettings />);

    expect(screen.getByTestId("github-app-callback-result").textContent).toContain(
      "GitHub App setup failed",
    );
    fireEvent.click(screen.getByTestId("github-app-permissions-button"));
    await waitFor(() => expect(screen.getByTestId("github-app-permissions-list")).toBeTruthy());
    expect(screen.getByText("Read and write repository content")).toBeTruthy();
  });

  it("associates public URL validation errors with the input", () => {
    setStatus({
      source: "none",
      state: "unconfigured",
      ready: false,
      read_only: false,
    });
    render(<GitHubAppSettings />);

    fireEvent.change(screen.getByLabelText("Organization login"), { target: { value: "acme" } });
    fireEvent.change(screen.getByLabelText("Public Kandev URL"), {
      target: { value: "http://localhost:8080" },
    });
    fireEvent.click(screen.getByTestId("github-app-create-button"));

    const input = screen.getByLabelText("Public Kandev URL");
    expect(screen.getByText("Enter a public HTTPS origin.").getAttribute("id")).toBe(
      "github-app-public-url-error",
    );
    expect(input.getAttribute("aria-describedby")).toBe(
      "github-app-public-url-help github-app-public-url-error",
    );
    expect(input.getAttribute("aria-errormessage")).toBe("github-app-public-url-error");
  });
});
