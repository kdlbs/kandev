import { act, cleanup, fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { StateProvider, useAppStoreApi } from "@/components/state-provider";
import { ToastProvider } from "@/components/toast-provider";
import type { SecretListItem } from "@/lib/types/http-secrets";
import { registerSecretsHandlers } from "@/lib/ws/handlers/secrets";
import { SettingsSaveProvider } from "./settings-save-provider";
import { SecretsSettings } from "./secrets-settings";

const TIMESTAMP = "2026-10-10T00:00:00Z";
const saved: SecretListItem = {
  id: "saved-global",
  name: "Accepted global metadata",
  scope: "global",
  has_value: true,
  created_at: TIMESTAMP,
  updated_at: TIMESTAMP,
};
const existing = { ...saved, id: "existing-global", name: "Existing global metadata" };
const pending: Array<() => void> = [];

function jsonResponse(items: SecretListItem | SecretListItem[]) {
  return new Response(JSON.stringify(items), {
    status: 200,
    headers: { "Content-Type": "application/json" },
  });
}

function externalHttp() {
  let resolve!: (response: Response) => void;
  const initialGet = new Promise<Response>((done) => {
    resolve = done;
  });
  pending.push(() => resolve(jsonResponse([])));
  const requests: Array<{ path: string; method: string; body: unknown }> = [];
  const unexpected: string[] = [];
  vi.stubGlobal(
    "fetch",
    vi.fn((input: RequestInfo | URL, init?: RequestInit) => {
      const path = new URL(String(input), "http://localhost").pathname;
      const method = init?.method ?? "GET";
      const body = typeof init?.body === "string" ? JSON.parse(init.body) : undefined;
      requests.push({ path, method, body });
      if (path === "/api/v1/secrets" && method === "GET") return initialGet;
      if (path === "/api/v1/secrets" && method === "POST")
        return Promise.resolve(jsonResponse(saved));
      unexpected.push(`${method} ${path}`);
      return Promise.reject(new Error(`Unexpected transport: ${method} ${path}`));
    }),
  );
  return { requests, unexpected, resolve };
}

function renderSettings(items: SecretListItem[] = [], loaded = false) {
  let store!: ReturnType<typeof useAppStoreApi>;
  function CaptureStore() {
    store = useAppStoreApi();
    return null;
  }
  const view = render(
    <StateProvider initialState={{ secrets: { items, loaded, loading: false } }}>
      <ToastProvider>
        <SettingsSaveProvider>
          <CaptureStore />
          <SecretsSettings />
        </SettingsSaveProvider>
      </ToastProvider>
    </StateProvider>,
  );
  return { ...view, store };
}

function expectRow(item: SecretListItem) {
  const row = screen.queryByTestId(`secret-row-${item.id}`);
  expect(row).not.toBeNull();
  expect(within(screen.getByTestId(`secret-row-${item.id}`)).getByText(item.name)).not.toBeNull();
}

async function createThroughSettings(
  http: ReturnType<typeof externalHttp>,
  store: ReturnType<typeof useAppStoreApi>,
) {
  fireEvent.click(screen.getByRole("button", { name: "Add secret" }));
  fireEvent.change(screen.getByPlaceholderText("Name (e.g. OpenAI Production Key)"), {
    target: { value: saved.name },
  });
  fireEvent.change(screen.getByPlaceholderText("Secret value"), {
    target: { value: "synthetic-test-value" },
  });
  const save = await screen.findByRole("button", { name: "Save changes" });
  await waitFor(() => expect((save as HTMLButtonElement).disabled).toBe(false));
  fireEvent.click(save);
  await waitFor(() => expect(store.getState().secrets.items).toContainEqual(saved));
  expect(http.requests.filter(({ method }) => method === "POST")).toEqual([
    {
      path: "/api/v1/secrets",
      method: "POST",
      body: { name: saved.name, value: "synthetic-test-value", scope: "global" },
    },
  ]);
  expectRow(saved);
  act(() => {
    registerSecretsHandlers(store)["secrets.created"]!({
      type: "notification",
      action: "secrets.created",
      payload: saved,
      timestamp: TIMESTAMP,
    });
  });
  expectRow(saved);
}

afterEach(async () => {
  await act(async () => {
    pending.splice(0).forEach((resolve) => resolve());
  });
  cleanup();
  vi.unstubAllGlobals();
});

// @covers AC-WORKSPACES-REPOSITORY-SECRETS-002.1
// @covers AC-WORKSPACES-REPOSITORY-SECRETS-002.3
// @covers AC-WORKSPACES-REPOSITORY-SECRETS-001.1
// @covers AC-WORKSPACES-REPOSITORY-SECRETS-001.2
describe("real Global Settings save publication", () => {
  it("keeps an accepted saved global row after an older initial GET succeeds empty", async () => {
    const http = externalHttp();
    const { store } = renderSettings();
    expect(store.getState().secrets).toMatchObject({ loaded: false, loading: true });
    await createThroughSettings(http, store);
    expect(store.getState().secrets.items).toEqual([saved]);
    expect(store.getState().secrets).toMatchObject({ loaded: false, loading: true });
    await act(async () => {
      http.resolve(jsonResponse([]));
    });
    expect(store.getState().secrets.items).toEqual([saved]);
    expectRow(saved);
    expect(store.getState().secrets).toMatchObject({ loaded: true, loading: false });
    expect(http.requests.filter(({ method }) => method === "GET")).toHaveLength(1);
    expect(http.unexpected).toEqual([]);
  });

  it("renders ordinary initial GET rows", async () => {
    const http = externalHttp();
    const { store } = renderSettings();
    await act(async () => {
      http.resolve(jsonResponse([existing]));
    });
    expectRow(existing);
    expect(store.getState().secrets).toMatchObject({
      items: [existing],
      loaded: true,
      loading: false,
    });
    expect(http.requests.filter(({ method }) => method === "GET")).toHaveLength(1);
    expect(http.unexpected).toEqual([]);
  });

  it("saves on an already-loaded global list without an initial GET", async () => {
    const http = externalHttp();
    const { store } = renderSettings([existing], true);
    expectRow(existing);
    await createThroughSettings(http, store);
    expect(store.getState().secrets.items).toEqual([existing, saved]);
    expectRow(existing);
    expectRow(saved);
    expect(http.requests.filter(({ method }) => method === "GET")).toEqual([]);
    expect(http.unexpected).toEqual([]);
  });
});
