import { afterEach, describe, expect, it, vi } from "vitest";
import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { TooltipProvider } from "@kandev/ui/tooltip";
import type { SentryConfig } from "@/lib/types/sentry";

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
      <SentryInstanceForm
        workspaceId="workspace-1"
        instance={savedInstance}
        idPrefix="sentry-edit"
        onSaved={vi.fn()}
        onCancel={vi.fn()}
      />
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

  it("labels new-instance forms without labeling edit forms", () => {
    render(
      <TooltipProvider>
        <SentryInstanceForm
          workspaceId="workspace-1"
          instance={null}
          idPrefix="sentry-add"
          onSaved={vi.fn()}
          onCancel={vi.fn()}
        />
      </TooltipProvider>,
    );

    expect(screen.getByTestId("sentry-add-form-heading").textContent).toBe("New Instance");
    expect(screen.getByTestId("sentry-add-form-heading").className).toContain("font-semibold");
    expect(screen.queryByTestId("sentry-edit-form-heading")).toBeNull();
  });
});
