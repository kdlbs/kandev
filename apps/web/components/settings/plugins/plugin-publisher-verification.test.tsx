import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { PluginRecord } from "@/lib/types/plugins";
import { PluginPublisherVerification } from "./plugin-publisher-verification";

afterEach(() => cleanup());

const VERIFY_BUTTON_TEST_ID = "plugin-verify-publisher";

function plugin(overrides: Partial<PluginRecord> = {}): PluginRecord {
  return {
    id: "acme",
    api_version: 1,
    version: "1.0.0",
    display_name: "Acme",
    description: "",
    author: "kandev",
    categories: [],
    capabilities: {},
    status: "active",
    install_path: "/plugins/acme/1.0.0",
    installation_id: "install-1",
    signed: false,
    installed_at: "2026-01-01T00:00:00Z",
    restart_count: 0,
    publisher_identity: { status: "unverified" },
    publisher_provenance: {
      origin: "upload",
      package_id: "acme",
      version: "1.0.0",
    },
    ...overrides,
  };
}

describe("PluginPublisherVerification", () => {
  it("offers an admin verification action and keeps the upload origin visible", () => {
    const onVerify = vi.fn();
    render(
      <PluginPublisherVerification
        plugin={plugin()}
        canManage
        busy={false}
        success={false}
        onVerify={onVerify}
      />,
    );

    expect(screen.getByText("Unverified publisher")).toBeTruthy();
    expect(screen.getByText("Uploaded file")).toBeTruthy();
    fireEvent.click(screen.getByTestId(VERIFY_BUTTON_TEST_ID));
    expect(onVerify).toHaveBeenCalledTimes(1);
  });

  it("announces progress and retryable errors", () => {
    const onVerify = vi.fn();
    const { rerender } = render(
      <PluginPublisherVerification
        plugin={plugin()}
        canManage
        busy
        success={false}
        onVerify={onVerify}
      />,
    );
    expect(screen.getByText("Verifying installed files…")).toBeTruthy();
    expect((screen.getByTestId(VERIFY_BUTTON_TEST_ID) as HTMLButtonElement).disabled).toBe(true);

    rerender(
      <PluginPublisherVerification
        plugin={plugin()}
        canManage
        busy={false}
        error="No trusted package is available for this installed version."
        success={false}
        onVerify={onVerify}
      />,
    );
    fireEvent.click(screen.getByText("Retry verification"));
    expect(onVerify).toHaveBeenCalledTimes(1);
  });

  it("shows the matched official source after success", () => {
    render(
      <PluginPublisherVerification
        plugin={plugin({
          publisher_identity: {
            status: "verified",
            login: "acme",
            matched_source: "official",
          },
        })}
        canManage
        busy={false}
        success
        onVerify={vi.fn()}
      />,
    );

    expect(screen.getByText("Publisher: acme")).toBeTruthy();
    expect(screen.getByText("Verified publisher")).toBeTruthy();
    expect(screen.getByText("Kandev Official")).toBeTruthy();
    expect(screen.getByText("Publisher verified for this installed version.")).toBeTruthy();
    expect(screen.queryByTestId(VERIFY_BUTTON_TEST_ID)).toBeNull();
  });

  it("shows trust information without exposing a verification control to readers", () => {
    render(
      <PluginPublisherVerification
        plugin={plugin()}
        canManage={false}
        busy={false}
        success={false}
        onVerify={vi.fn()}
      />,
    );

    expect(screen.getByText("Unverified publisher")).toBeTruthy();
    expect(screen.getByText("Declared author:")).toBeTruthy();
    expect(screen.getByText("kandev")).toBeTruthy();
    expect(screen.queryByTestId(VERIFY_BUTTON_TEST_ID)).toBeNull();
  });
});
