import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, beforeAll, describe, expect, it, vi } from "vitest";

import { activateLocale } from "@/lib/i18n";
import { OAuthFields } from "./jira-oauth-fields";

const api = vi.hoisted(() => ({
  start: vi.fn(),
  complete: vi.fn(),
}));
const toast = vi.hoisted(() => vi.fn());

vi.mock("@/lib/api/domains/jira-api", () => ({
  startJiraOAuth: api.start,
  completeJiraOAuth: api.complete,
}));
vi.mock("@/components/toast-provider", () => ({
  useToast: () => ({ toast }),
}));

beforeAll(async () => {
  await activateLocale("en");
});

afterEach(() => {
  cleanup();
  api.start.mockReset();
  api.complete.mockReset();
  toast.mockReset();
  vi.restoreAllMocks();
});

function renderOAuth(onConnected = vi.fn()) {
  render(
    <OAuthFields
      form={{ siteUrl: "https://acme.atlassian.net" }}
      loading={false}
      workspaceId="ws-1"
      connected={false}
      onConnected={onConnected}
    />,
  );
  return onConnected;
}

describe("OAuthFields callback handoff", () => {
  it("completes an origin-checked message from the authorization popup", async () => {
    api.start.mockResolvedValue({
      authUrl: "https://auth.example/authorize?client_id=client-1&state=state-1",
    });
    api.complete.mockResolvedValue({ ok: true });
    const popup = { location: { assign: vi.fn() }, close: vi.fn() };
    vi.spyOn(window, "open").mockReturnValue(popup as unknown as Window);
    const onConnected = renderOAuth();

    fireEvent.click(screen.getByTestId("jira-oauth-connect"));
    await waitFor(() => expect(api.start).toHaveBeenCalled());
    window.dispatchEvent(
      new MessageEvent("message", {
        origin: window.location.origin,
        source: popup as unknown as WindowProxy,
        data: { type: "jira-oauth-callback", code: "code-1", state: "state-1" },
      }),
    );

    await waitFor(() => expect(api.complete).toHaveBeenCalledWith("code-1", "state-1"));
    expect(onConnected).toHaveBeenCalledOnce();
  });

  it("does not start OAuth when the browser blocks the popup", async () => {
    vi.spyOn(window, "open").mockReturnValue(null);
    renderOAuth();

    fireEvent.click(screen.getByTestId("jira-oauth-connect"));

    await waitFor(() => expect(toast).toHaveBeenCalled());
    expect(api.start).not.toHaveBeenCalled();
  });
});
