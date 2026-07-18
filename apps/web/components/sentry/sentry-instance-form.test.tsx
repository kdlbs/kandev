import { afterEach, describe, expect, it, vi } from "vitest";
import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { TooltipProvider } from "@kandev/ui/tooltip";
import type { SentryConfig } from "@/lib/types/sentry";
import { SettingsSaveProvider } from "@/components/settings/settings-save-provider";

vi.mock("@/components/toast-provider", () => ({
  useToast: () => ({ toast: vi.fn() }),
}));

vi.mock("@/lib/api/domains/sentry-api", () => ({
  createSentryInstance: vi.fn(),
  updateSentryInstance: vi.fn(),
  testSentryConnection: vi.fn(),
  testSentryInstance: vi.fn(),
  sentryErrorCode: vi.fn(),
  SENTRY_ERROR_CODES: { nameTaken: "SENTRY_INSTANCE_NAME_TAKEN" },
}));

import { SentryInstanceForm } from "./sentry-instance-form";
import { createSentryInstance, updateSentryInstance } from "@/lib/api/domains/sentry-api";

const savedInstance: SentryConfig = {
  id: "instance-1",
  workspaceId: "workspace-1",
  name: "Production",
  authMethod: "auth_token",
  url: "https://sentry.example.com",
  hasSecret: true,
  lastOk: true,
  createdAt: "2026-01-01T00:00:00Z",
  updatedAt: "2026-01-01T00:00:00Z",
};

function renderForm() {
  return render(
    <TooltipProvider>
      <SettingsSaveProvider>
        <SentryInstanceForm
          workspaceId="workspace-1"
          instance={savedInstance}
          idPrefix="sentry-edit"
          onSaved={vi.fn()}
          onCancel={vi.fn()}
        />
      </SettingsSaveProvider>
    </TooltipProvider>,
  );
}

afterEach(() => {
  cleanup();
  vi.clearAllMocks();
});

describe("SentryInstanceForm", () => {
  it("requires a token before testing an edited saved URL", () => {
    renderForm();

    fireEvent.change(screen.getByTestId("sentry-edit-url-input"), {
      target: { value: "https://sentry-new.example.com" },
    });

    expect(screen.getByTestId("sentry-edit-test-button")).toHaveProperty("disabled", true);
  });

  it("persists existing instance edits only through the shared save action", async () => {
    vi.mocked(updateSentryInstance).mockResolvedValue(savedInstance);
    renderForm();

    fireEvent.change(screen.getByTestId("sentry-edit-name-input"), {
      target: { value: "Primary" },
    });
    expect(updateSentryInstance).not.toHaveBeenCalled();
    expect(screen.queryByTestId("sentry-edit-save-button")).toBeNull();

    fireEvent.click(screen.getByRole("button", { name: "Save changes" }));
    await waitFor(() =>
      expect(updateSentryInstance).toHaveBeenCalledWith(
        "workspace-1",
        savedInstance.id,
        expect.objectContaining({ name: "Primary" }),
      ),
    );
  });

  it("labels new-instance forms without labeling edit forms", () => {
    render(
      <TooltipProvider>
        <SettingsSaveProvider>
          <SentryInstanceForm
            workspaceId="workspace-1"
            instance={null}
            idPrefix="sentry-add"
            onSaved={vi.fn()}
            onCancel={vi.fn()}
          />
        </SettingsSaveProvider>
      </TooltipProvider>,
    );

    expect(screen.getByTestId("sentry-add-form-heading").textContent).toBe("New Instance");
    expect(screen.getByTestId("sentry-add-form-heading").className).toContain("font-semibold");
    expect(screen.queryByTestId("sentry-edit-form-heading")).toBeNull();
  });

  it("treats a new instance as dirty and creates it through the shared save action", async () => {
    vi.mocked(createSentryInstance).mockResolvedValue(savedInstance);
    render(
      <TooltipProvider>
        <SettingsSaveProvider>
          <SentryInstanceForm
            workspaceId="workspace-1"
            instance={null}
            idPrefix="sentry-add"
            onSaved={vi.fn()}
            onCancel={vi.fn()}
          />
        </SettingsSaveProvider>
      </TooltipProvider>,
    );

    expect(screen.getByTestId("sentry-add-form").getAttribute("data-settings-dirty")).toBe("true");
    expect(screen.queryByTestId("sentry-add-save-button")).toBeNull();
    fireEvent.change(screen.getByTestId("sentry-add-name-input"), {
      target: { value: "Production" },
    });
    fireEvent.change(screen.getByTestId("sentry-add-secret-input"), {
      target: { value: "sntrys_token" },
    });
    expect(createSentryInstance).not.toHaveBeenCalled();

    fireEvent.click(screen.getByRole("button", { name: "Save changes" }));
    await waitFor(() =>
      expect(createSentryInstance).toHaveBeenCalledWith(
        "workspace-1",
        expect.objectContaining({ name: "Production", secret: "sntrys_token" }),
      ),
    );
  });
});
