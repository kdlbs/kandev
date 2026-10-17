import { act, cleanup, fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import { expect, vi } from "vitest";
import { StateProvider, useAppStoreApi } from "@/components/state-provider";
import { ToastProvider } from "@/components/toast-provider";
import type { SecretListItem } from "@/lib/types/http-secrets";
import { registerSecretsHandlers } from "@/lib/ws/handlers/secrets";
import { SettingsSaveProvider } from "./settings-save-provider";
import { SecretsSettings } from "./secrets-settings";

export const acceptedSecret: SecretListItem = {
  id: "server-error-accepted",
  name: "Saved while initial metadata is pending",
  scope: "global",
  has_value: true,
  created_at: "2026-10-12T20:00:00Z",
  updated_at: "2026-10-12T20:00:00Z",
};
export const existingSecret = { ...acceptedSecret, id: "prior-secret", name: "Prior metadata" };
const syntheticValue = "test-only-synthetic-credential";
const pendingReads = new Set<() => void>();

export function holdGlobalRequests() {
  let resolve!: (response: Response) => void;
  const held = new Promise<Response>((done) => {
    resolve = done;
  });
  const release = (status: number, body: unknown) => {
    pendingReads.delete(cleanupRead);
    resolve(
      new Response(JSON.stringify(body), {
        status,
        headers: { "Content-Type": "application/json" },
      }),
    );
  };
  const cleanupRead = () => release(200, []);
  pendingReads.add(cleanupRead);
  const requests: Array<{ method: string; path: string; body: unknown }> = [];
  const unexpected: string[] = [];
  vi.stubGlobal(
    "fetch",
    vi.fn((input: RequestInfo | URL, init?: RequestInit) => {
      const path = new URL(String(input), "http://localhost").pathname;
      const method = init?.method ?? "GET";
      const body = typeof init?.body === "string" ? JSON.parse(init.body) : undefined;
      requests.push({ method, path, body });
      if (path === "/api/v1/secrets" && method === "GET") return held;
      if (path === "/api/v1/secrets" && method === "POST") {
        return Promise.resolve(
          new Response(JSON.stringify(acceptedSecret), {
            status: 200,
            headers: { "Content-Type": "application/json" },
          }),
        );
      }
      unexpected.push(`${method} ${path}`);
      return Promise.reject(new Error(`Unexpected secret transport: ${method} ${path}`));
    }),
  );
  return { release, requests, unexpected };
}

export function mountGlobalSettings(items: SecretListItem[] = [], loaded = false) {
  let store!: ReturnType<typeof useAppStoreApi>;
  function StoreObserver() {
    store = useAppStoreApi();
    return null;
  }
  const mounted = render(
    <StateProvider initialState={{ secrets: { items, loaded, loading: false } }}>
      <ToastProvider>
        <SettingsSaveProvider>
          <StoreObserver />
          <SecretsSettings />
        </SettingsSaveProvider>
      </ToastProvider>
    </StateProvider>,
  );
  return { store, ...mounted };
}

export function expectMetadataRow(item: SecretListItem) {
  const row = screen.queryByTestId(`secret-row-${item.id}`);
  expect(row).not.toBeNull();
  expect(within(screen.getByTestId(`secret-row-${item.id}`)).getByText(item.name)).not.toBeNull();
}

export async function acceptThroughSave(
  store: ReturnType<typeof useAppStoreApi>,
  http: ReturnType<typeof holdGlobalRequests>,
) {
  fireEvent.click(screen.getByRole("button", { name: "Add secret" }));
  fireEvent.change(screen.getByPlaceholderText("Name (e.g. OpenAI Production Key)"), {
    target: { value: acceptedSecret.name },
  });
  fireEvent.change(screen.getByPlaceholderText("Secret value"), {
    target: { value: syntheticValue },
  });
  const save = await screen.findByRole("button", { name: "Save changes" });
  await waitFor(() => expect((save as HTMLButtonElement).disabled).toBe(false));
  fireEvent.click(save);
  await waitFor(() => expect(store.getState().secrets.items).toContainEqual(acceptedSecret));
  expect(http.requests.filter((request) => request.method === "POST")).toEqual([
    {
      method: "POST",
      path: "/api/v1/secrets",
      body: { name: acceptedSecret.name, value: syntheticValue, scope: "global" },
    },
  ]);
  expectMetadataRow(acceptedSecret);
  act(() =>
    registerSecretsHandlers(store)["secrets.created"]!({
      id: "accepted-create-echo",
      type: "notification",
      action: "secrets.created",
      payload: acceptedSecret,
      timestamp: acceptedSecret.updated_at,
    }),
  );
  expect(store.getState().secrets.items.filter((item) => item.id === acceptedSecret.id)).toEqual([
    acceptedSecret,
  ]);
  expectMetadataRow(acceptedSecret);
}

export async function finishSettingsRequests() {
  await act(async () => {
    for (const complete of pendingReads) complete();
  });
  cleanup();
  vi.unstubAllGlobals();
}
