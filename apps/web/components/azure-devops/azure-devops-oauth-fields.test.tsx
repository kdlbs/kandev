import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, beforeAll, describe, expect, it, vi } from "vitest";

import { activateLocale } from "@/lib/i18n";
import type { AzureDevOpsConfig } from "@/lib/types/azure-devops";

import { AzureDevOpsOAuthFields } from "./azure-devops-oauth-fields";
import { parseOAuthRedirect } from "@/hooks/domains/azure-devops/use-azure-devops-oauth-connect";

const api = vi.hoisted(() => ({ start: vi.fn(), complete: vi.fn() }));
const toast = vi.hoisted(() => vi.fn());

vi.mock("@/lib/api/domains/azure-devops-oauth-api", () => ({
  startAzureDevOpsOAuth: api.start,
  completeAzureDevOpsOAuth: api.complete,
}));
vi.mock("@/components/toast-provider", () => ({
  useToast: () => ({ toast }),
}));

const REQUEST = { organizationUrl: "https://dev.azure.com/acme", defaultProjectId: "p-1" };
const CONNECTED: AzureDevOpsConfig = {
  workspaceId: "ws-1",
  organizationUrl: "https://dev.azure.com/acme",
  authMethod: "oauth",
  hasSecret: true,
  lastOk: true,
  oauthAccount: "Ana Silva",
  createdAt: "2026-01-01T00:00:00Z",
  updatedAt: "2026-01-01T00:00:00Z",
};
const AZURE_CLI_CLIENT_ID = "04b07795-8ddb-461a-bbee-02f9e1bf7b46";
const CONNECT_TEST_ID = "azure-devops-oauth-connect";
const PASTE_TEST_ID = "azure-devops-oauth-paste";
const DEFAULT_CLIENT_ID = "11111111-1111-1111-1111-111111111111";

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

function renderFields(props: Partial<Parameters<typeof AzureDevOpsOAuthFields>[0]> = {}) {
  const onConnected = vi.fn();
  render(
    <AzureDevOpsOAuthFields
      workspaceId="ws-1"
      config={null}
      request={REQUEST}
      clientId=""
      defaultClientId={DEFAULT_CLIENT_ID}
      onClientIdChange={vi.fn()}
      onSignOut={vi.fn()}
      signingOut={false}
      signedIn={false}
      organizationValid
      disabled={false}
      onConnected={onConnected}
      {...props}
    />,
  );
  return onConnected;
}

async function startSignIn() {
  api.start.mockResolvedValue({ authUrl: "https://login.example/authorize?state=s1", state: "s1" });
  const popup = { location: { assign: vi.fn() }, close: vi.fn(), closed: false };
  vi.spyOn(window, "open").mockReturnValue(popup as unknown as Window);
  fireEvent.click(screen.getByTestId(CONNECT_TEST_ID));
  await waitFor(() => expect(popup.location.assign).toHaveBeenCalled());
  return popup;
}

function postFromPopup(
  popup: unknown,
  data: Record<string, string>,
  origin = window.location.origin,
) {
  window.dispatchEvent(
    new MessageEvent("message", {
      origin,
      source: popup as WindowProxy,
      data: { type: "azure-devops-oauth-callback", code: "", state: "", error: "", ...data },
    }),
  );
}

describe("AzureDevOpsOAuthFields", () => {
  it("opens Microsoft sign-in in a popup and completes from its callback", async () => {
    api.complete.mockResolvedValue(CONNECTED);
    const onConnected = renderFields();

    const popup = await startSignIn();
    expect(api.start).toHaveBeenCalledWith("ws-1", REQUEST);
    expect(popup.location.assign).toHaveBeenCalledWith("https://login.example/authorize?state=s1");
    expect(screen.getByTestId(PASTE_TEST_ID)).toBeTruthy();

    postFromPopup(popup, { code: "code-1", state: "s1" });

    await waitFor(() =>
      expect(api.complete).toHaveBeenCalledWith("ws-1", { code: "code-1", state: "s1" }),
    );
    await waitFor(() => expect(onConnected).toHaveBeenCalledWith(CONNECTED));
    expect(screen.queryByTestId(PASTE_TEST_ID)).toBeNull();
  });

  it("ignores messages from another origin, another window, or another attempt", async () => {
    renderFields();
    const popup = await startSignIn();

    postFromPopup(popup, { code: "c", state: "s1" }, "https://evil.example");
    postFromPopup({}, { code: "c", state: "s1" });
    postFromPopup(popup, { code: "c", state: "other" });

    await new Promise((resolve) => setTimeout(resolve, 0));
    expect(api.complete).not.toHaveBeenCalled();
  });

  it("reports a declined sign-in without completing", async () => {
    renderFields();
    const popup = await startSignIn();

    postFromPopup(popup, { state: "s1", error: "access_denied" });

    await waitFor(() => expect(toast).toHaveBeenCalled());
    expect(api.complete).not.toHaveBeenCalled();
    expect(popup.close).toHaveBeenCalled();
    expect(screen.queryByTestId(PASTE_TEST_ID)).toBeNull();
  });

  it("completes from a pasted redirect address and rejects incomplete ones", async () => {
    api.complete.mockResolvedValue(CONNECTED);
    const onConnected = renderFields();
    await startSignIn();
    const input = screen.getByTestId("azure-devops-oauth-paste-url");

    fireEvent.change(input, { target: { value: "http://localhost/cb?state=s1" } });
    fireEvent.click(screen.getByTestId("azure-devops-oauth-paste-complete"));
    expect(api.complete).not.toHaveBeenCalled();
    expect(toast).toHaveBeenCalled();

    fireEvent.change(input, {
      target: { value: "http://localhost/api/v1/azure-devops/oauth/callback?code=c2&state=s1" },
    });
    fireEvent.click(screen.getByTestId("azure-devops-oauth-paste-complete"));
    await waitFor(() => expect(onConnected).toHaveBeenCalledWith(CONNECTED));
    expect(api.complete).toHaveBeenCalledWith("ws-1", { code: "c2", state: "s1" });
  });

  it("completes when the popup returns to the Kandev origin without posting a message", async () => {
    api.complete.mockResolvedValue(CONNECTED);
    const onConnected = renderFields();
    const popup = await startSignIn();
    const location = popup.location as unknown as { origin: string; search: string };

    location.origin = "https://login.microsoftonline.com";
    location.search = "?code=ignored&state=s1";
    await new Promise((resolve) => setTimeout(resolve, 400));
    expect(api.complete).not.toHaveBeenCalled();

    location.origin = window.location.origin;
    location.search = "?code=code-9&state=s1";
    await waitFor(() => expect(onConnected).toHaveBeenCalledWith(CONNECTED));
    expect(api.complete).toHaveBeenCalledTimes(1);
    expect(api.complete).toHaveBeenCalledWith("ws-1", { code: "code-9", state: "s1" });
    expect(popup.close).toHaveBeenCalled();
  });

  it("does not start when the browser blocks the popup", async () => {
    vi.spyOn(window, "open").mockReturnValue(null);
    renderFields();

    fireEvent.click(screen.getByTestId(CONNECT_TEST_ID));

    await waitFor(() => expect(toast).toHaveBeenCalled());
    expect(api.start).not.toHaveBeenCalled();
  });

  it("shows the signed-in identity and the re-sign-in state", () => {
    renderFields({ config: CONNECTED, signedIn: true });
    expect(screen.getByTestId("azure-devops-oauth-account").textContent).toContain("Ana Silva");
    cleanup();

    renderFields({ config: { ...CONNECTED, reauthRequired: true }, signedIn: true });
    expect(screen.getByTestId("azure-devops-oauth-reauth")).toBeTruthy();
    expect(screen.queryByTestId("azure-devops-oauth-account")).toBeNull();
  });

  it("keeps sign-in disabled until the organization URL is valid", () => {
    renderFields({ organizationValid: false });
    expect((screen.getByTestId(CONNECT_TEST_ID) as HTMLButtonElement).disabled).toBe(true);
  });
});

describe("AzureDevOpsOAuthFields application", () => {
  it("offers the installation default as placeholder, reports edits, and has no redirect field", () => {
    const onClientIdChange = vi.fn();
    renderFields({ onClientIdChange });
    const clientId = screen.getByTestId("azure-devops-oauth-client-id") as HTMLInputElement;
    expect(clientId.placeholder).toBe(DEFAULT_CLIENT_ID);
    expect(screen.queryByTestId("azure-devops-oauth-redirect-url")).toBeNull();

    fireEvent.change(clientId, { target: { value: AZURE_CLI_CLIENT_ID } });
    expect(onClientIdChange).toHaveBeenCalledWith(AZURE_CLI_CLIENT_ID);
  });

  it("requires a client ID when the installation has no default", () => {
    renderFields({ defaultClientId: "" });
    expect(screen.getByTestId("azure-devops-oauth-client-required")).toBeTruthy();
    expect((screen.getByTestId(CONNECT_TEST_ID) as HTMLButtonElement).disabled).toBe(true);
    cleanup();

    renderFields({ defaultClientId: "", clientId: AZURE_CLI_CLIENT_ID });
    expect(screen.queryByTestId("azure-devops-oauth-client-required")).toBeNull();
    expect((screen.getByTestId(CONNECT_TEST_ID) as HTMLButtonElement).disabled).toBe(false);
  });
});

describe("AzureDevOpsOAuthFields application in use", () => {
  const SIGNED_IN = { ...CONNECTED, oauthClientId: AZURE_CLI_CLIENT_ID };

  it("shows the application the connection uses", () => {
    renderFields({ config: SIGNED_IN, signedIn: true, clientId: SIGNED_IN.oauthClientId });
    expect(screen.getByTestId("azure-devops-oauth-app-in-use").textContent).toContain(
      AZURE_CLI_CLIENT_ID,
    );
    expect(screen.queryByTestId("azure-devops-oauth-app-pending")).toBeNull();
  });

  it("warns that a different application applies only on the next sign-in", () => {
    renderFields({ config: SIGNED_IN, signedIn: true, clientId: "" });
    expect(screen.getByTestId("azure-devops-oauth-app-pending")).toBeTruthy();
    cleanup();

    renderFields({
      config: SIGNED_IN,
      signedIn: true,
      clientId: "04B07795-8DDB-461A-BBEE-02F9E1BF7B46",
    });
    expect(screen.queryByTestId("azure-devops-oauth-app-pending")).toBeNull();
  });
});

describe("AzureDevOpsOAuthFields sign-out", () => {
  it("offers Sign out only while signed in", () => {
    const onSignOut = vi.fn();
    renderFields({ config: CONNECTED, signedIn: true, onSignOut });
    fireEvent.click(screen.getByTestId("azure-devops-oauth-sign-out"));
    expect(onSignOut).toHaveBeenCalledOnce();
    cleanup();

    renderFields({ config: null, signedIn: false });
    expect(screen.queryByTestId("azure-devops-oauth-sign-out")).toBeNull();
  });

  it("disables Sign out while it runs", () => {
    renderFields({ config: CONNECTED, signedIn: true, signingOut: true });
    expect((screen.getByTestId("azure-devops-oauth-sign-out") as HTMLButtonElement).disabled).toBe(
      true,
    );
  });
});

describe("AzureDevOpsOAuthFields completion guards", () => {
  it("completes once when the pasted address and the popup report the same result", async () => {
    let finish: (value: AzureDevOpsConfig) => void = () => undefined;
    api.complete.mockImplementation(
      () => new Promise<AzureDevOpsConfig>((resolve) => (finish = resolve)),
    );
    renderFields();
    const popup = await startSignIn();
    fireEvent.change(screen.getByTestId("azure-devops-oauth-paste-url"), {
      target: { value: "http://localhost/cb?code=code-1&state=s1" },
    });
    fireEvent.click(screen.getByTestId("azure-devops-oauth-paste-complete"));
    await waitFor(() => expect(api.complete).toHaveBeenCalledTimes(1));
    expect((screen.getByTestId(CONNECT_TEST_ID) as HTMLButtonElement).disabled).toBe(true);

    const location = popup.location as unknown as { origin: string; search: string };
    location.origin = window.location.origin;
    location.search = "?code=code-1&state=s1";
    postFromPopup(popup, { code: "code-1", state: "s1" });
    await new Promise((resolve) => setTimeout(resolve, 400));
    expect(api.complete).toHaveBeenCalledTimes(1);
    finish(CONNECTED);
  });

  it("stops watching a closed popup and keeps the paste fallback", async () => {
    renderFields();
    const popup = await startSignIn();
    popup.closed = true;
    await new Promise((resolve) => setTimeout(resolve, 400));
    const location = popup.location as unknown as { origin: string; search: string };
    location.origin = window.location.origin;
    location.search = "?code=late&state=s1";
    await new Promise((resolve) => setTimeout(resolve, 400));
    expect(api.complete).not.toHaveBeenCalled();
    expect(screen.getByTestId(PASTE_TEST_ID)).toBeTruthy();
  });

  it("announces progress without renaming the action", async () => {
    let started: (value: { authUrl: string; state: string }) => void = () => undefined;
    api.start.mockImplementation(() => new Promise((resolve) => (started = resolve)));
    vi.spyOn(window, "open").mockReturnValue({
      location: { assign: vi.fn() },
      close: vi.fn(),
      closed: false,
    } as unknown as Window);
    renderFields();
    const connect = screen.getByTestId(CONNECT_TEST_ID);
    const label = connect.textContent;
    fireEvent.click(connect);
    await waitFor(() => expect(screen.getByRole("status").textContent).not.toBe(""));
    expect(connect.textContent).toBe(label);
    started({ authUrl: "https://login.example/authorize?state=s1", state: "s1" });
  });
});

describe("parseOAuthRedirect", () => {
  it("reads code and state from a full redirect address", () => {
    expect(parseOAuthRedirect(" http://localhost:38429/cb?code=a&state=b ")).toEqual({
      code: "a",
      state: "b",
    });
    expect(parseOAuthRedirect("not a url")).toBeNull();
    expect(parseOAuthRedirect("http://localhost/cb?code=a")).toBeNull();
  });
});
