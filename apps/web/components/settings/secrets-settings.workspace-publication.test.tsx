import { act, cleanup, fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { StateProvider } from "@/components/state-provider";
import { ToastProvider } from "@/components/toast-provider";
import type { SecretListItem } from "@/lib/types/http-secrets";
import { SecretsSettings } from "./secrets-settings";
import { SettingsSaveProvider } from "./settings-save-provider";

const WORKSPACE = "workspace-publication-form";
const INITIAL_EMPTY: SecretListItem[] = [];
const saved: SecretListItem = {
  id: "accepted-workspace-secret",
  name: "Workspace deployment credential",
  scope: "workspace",
  workspace_id: WORKSPACE,
  has_value: true,
  created_at: "2026-10-10T08:00:00Z",
  updated_at: "2026-10-10T08:00:00Z",
};

function response(data: unknown) {
  return new Response(JSON.stringify(data), {
    status: 200,
    headers: { "Content-Type": "application/json" },
  });
}

let resolveList!: (value: Response) => void;
let listPromise: Promise<Response>;
const fetchTransport = vi.fn();
const requests: Array<{ method: string; url: URL; body?: unknown }> = [];

beforeEach(() => {
  requests.length = 0;
  fetchTransport.mockReset();
  listPromise = new Promise((resolve) => {
    resolveList = resolve;
  });
  fetchTransport.mockImplementation((input: string, init?: RequestInit) => {
    const url = new URL(input, "http://localhost");
    const method = init?.method ?? "GET";
    const body = typeof init?.body === "string" ? JSON.parse(init.body) : undefined;
    requests.push({ method, url, body });
    expect(url.pathname).toBe("/api/v1/secrets");
    if (method === "GET") {
      expect(url.searchParams.get("scope")).toBe("workspace");
      expect(url.searchParams.get("workspace_id")).toBe(WORKSPACE);
      return listPromise;
    }
    expect(method).toBe("POST");
    expect(body).toEqual({
      name: saved.name,
      value: "synthetic-workspace-input",
      scope: "workspace",
      workspace_id: WORKSPACE,
    });
    return Promise.resolve(response(saved));
  });
  vi.stubGlobal("fetch", fetchTransport);
});

afterEach(async () => {
  cleanup();
  await act(async () => {
    resolveList(response([]));
  });
  vi.unstubAllGlobals();
});

function mount(initialItems?: SecretListItem[]) {
  return render(
    <StateProvider>
      <ToastProvider>
        <SettingsSaveProvider>
          <SecretsSettings scope="workspace" workspaceId={WORKSPACE} initialItems={initialItems} />
        </SettingsSaveProvider>
      </ToastProvider>
    </StateProvider>,
  );
}

function expectSavedRow() {
  const row = screen.getByTestId(`secret-row-${saved.id}`);
  expect(within(row).getByText(saved.name)).not.toBeNull();
  expect(screen.queryAllByTestId(/^secret-row-/)).toHaveLength(1);
}

async function submitCreation() {
  fireEvent.click(screen.getByRole("button", { name: "Add secret" }));
  fireEvent.change(screen.getByPlaceholderText("Name (e.g. OpenAI Production Key)"), {
    target: { value: saved.name },
  });
  fireEvent.change(screen.getByPlaceholderText("Secret value"), {
    target: { value: "synthetic-workspace-input" },
  });
  const save = screen.getByRole("button", { name: "Save changes" });
  await waitFor(() => expect((save as HTMLButtonElement).disabled).toBe(false));
  fireEvent.click(save);
  await screen.findByTestId(`secret-row-${saved.id}`);
  expectSavedRow();
}

// @covers AC-WORKSPACES-SECRET-CATALOGUE-001.1
// @covers AC-WORKSPACES-SECRET-CATALOGUE-001.3
// @covers AC-WORKSPACES-REPOSITORY-SECRETS-001.1
describe("workspace initial publication through the real settings form", () => {
  it("keeps the accepted workspace row after the earlier empty initial GET", async () => {
    mount();
    expect(requests.map((request) => request.method)).toEqual(["GET"]);
    await submitCreation();
    expect(requests.map((request) => request.method)).toEqual(["GET", "POST"]);
    await act(async () => {
      resolveList(response([]));
    });
    expectSavedRow();
    expect(requests.map((request) => request.method)).toEqual(["GET", "POST"]);
  });

  it("renders the ordinary initial GET metadata row", async () => {
    mount();
    await act(async () => {
      resolveList(response([saved]));
    });
    expectSavedRow();
    expect(requests.map((request) => request.method)).toEqual(["GET"]);
  });

  it("keeps an accepted create with supplied empty initial items and no GET", async () => {
    mount(INITIAL_EMPTY);
    expect(requests).toEqual([]);
    await submitCreation();
    expectSavedRow();
    expect(requests.map((request) => request.method)).toEqual(["POST"]);
  });
});
