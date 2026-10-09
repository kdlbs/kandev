import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { WebhookConfig } from "./webhook-config";

vi.mock("@/lib/api/domains/automation-api", () => ({
  revealWebhookSecret: vi.fn().mockResolvedValue({ webhook_secret: "s3cret" }),
}));

afterEach(cleanup);

function renderConfig(config: Record<string, unknown>, automationId: string | null = null) {
  const onUpdate = vi.fn();
  render(
    <WebhookConfig
      automationId={automationId}
      workspaceId="ws-1"
      config={config}
      onUpdate={onUpdate}
    />,
  );
  return onUpdate;
}

describe("WebhookConfig admission fields", () => {
  it("reflects the configured dedup key and commits edits", () => {
    const onUpdate = renderConfig({ dedup_key: "issue.id" });

    const input = screen.getByDisplayValue("issue.id");
    fireEvent.change(input, { target: { value: "alert.fingerprint" } });

    expect(onUpdate).toHaveBeenCalledWith(
      expect.objectContaining({ dedup_key: "alert.fingerprint" }),
    );
  });

  it("adds a filter through the embedded filters list, preserving other config", () => {
    const onUpdate = renderConfig({ dedup_key: "issue.id", filters: [] });

    fireEvent.click(screen.getByText("Add filter"));

    expect(onUpdate).toHaveBeenCalledWith({
      dedup_key: "issue.id",
      filters: [{ path: "", op: "eq", values: [] }],
    });
  });

  it("reflects the configured repository selector and commits edits", () => {
    const onUpdate = renderConfig({ repository: { selector_path: "service" } });

    const input = screen.getByDisplayValue("service");
    fireEvent.change(input, { target: { value: "project" } });

    expect(onUpdate).toHaveBeenCalledWith(
      expect.objectContaining({ repository: { selector_path: "project" } }),
    );
  });

  it("clears the repository selector to undefined when emptied", () => {
    const onUpdate = renderConfig({ repository: { selector_path: "service" } });

    const input = screen.getByDisplayValue("service");
    fireEvent.change(input, { target: { value: "" } });

    expect(onUpdate).toHaveBeenCalledWith(expect.objectContaining({ repository: undefined }));
  });

  it("renders the admission fields before the automation is saved", async () => {
    renderConfig({ dedup_key: "issue.id" }, null);

    expect(await screen.findByDisplayValue("issue.id")).toBeInstanceOf(HTMLInputElement);
  });
});
