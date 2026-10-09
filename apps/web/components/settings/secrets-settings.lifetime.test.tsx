import { act, cleanup, fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { StateProvider } from "@/components/state-provider";
import { ToastProvider } from "@/components/toast-provider";
import { useSecrets } from "@/hooks/domains/settings/use-secrets";
import type { SecretListOptions } from "@/lib/api/domains/secrets-api";
import type { SecretListItem } from "@/lib/types/http-secrets";
import { SettingsSaveProvider } from "./settings-save-provider";
import { SecretsSettings } from "./secrets-settings";

const transport = vi.hoisted(() => ({
  list: vi.fn(),
  create: vi.fn(),
  update: vi.fn(),
  remove: vi.fn(),
  references: vi.fn(),
}));
vi.mock("@/lib/api/domains/secrets-api", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@/lib/api/domains/secrets-api")>()),
  listSecrets: transport.list,
  createSecret: transport.create,
  updateSecret: transport.update,
  deleteSecret: transport.remove,
  listSecretReferences: transport.references,
}));

const WORKSPACE_ID = "workspace-a";
const NAME_PLACEHOLDER = "Name (e.g. OpenAI Production Key)";
const SOURCES = ["supplied", "fetched"] as const;
const MUTATIONS = ["add", "update", "remove"] as const;
const OUTCOMES = ["success", "failure"] as const;
type Mutation = (typeof MUTATIONS)[number];

const original: SecretListItem = {
  id: "original",
  name: "Original metadata",
  scope: "workspace",
  workspace_id: WORKSPACE_ID,
  has_value: true,
  created_at: "2026-10-05T00:00:00Z",
  updated_at: "2026-10-05T00:00:00Z",
};
const created = { ...original, id: "created", name: "Created metadata" };
const renamed = { ...original, name: "Renamed metadata" };
const globalItem: SecretListItem = {
  ...original,
  id: "global",
  name: "Global metadata",
  scope: "global",
  workspace_id: undefined,
};

const pending: Array<() => void> = [];
function deferred<T>(cleanupValue: T) {
  let resolve!: (value: T) => void;
  let reject!: (error: Error) => void;
  const promise = new Promise<T>((yes, no) => {
    resolve = yes;
    reject = no;
  });
  pending.push(() => resolve(cleanupValue));
  return { promise, resolve, reject };
}

async function acknowledge<T>(request: ReturnType<typeof deferred<T>>, value: T) {
  await act(async () => request.resolve(value));
}

function GlobalConsumer() {
  const { items, loaded, loading } = useSecrets();
  return (
    <output data-testid="global-secrets-snapshot">
      {JSON.stringify({ items: items.map((item) => item.id), loaded, loading })}
    </output>
  );
}

function SettingsTree({
  initialItems,
  globalEnabled = false,
}: {
  initialItems?: SecretListItem[];
  globalEnabled?: boolean;
}) {
  return (
    <StateProvider>
      <ToastProvider>
        <SettingsSaveProvider>
          <SecretsSettings
            scope="workspace"
            workspaceId={WORKSPACE_ID}
            initialItems={initialItems}
          />
          {globalEnabled && <GlobalConsumer />}
        </SettingsSaveProvider>
      </ToastProvider>
    </StateProvider>
  );
}

function expectMetadataRows(expected: SecretListItem[]) {
  expect(screen.queryAllByTestId(/^secret-row-/).map((row) => row.dataset.testid)).toEqual(
    expected.map((item) => `secret-row-${item.id}`),
  );
  expected.forEach((item) => {
    expect(within(screen.getByTestId(`secret-row-${item.id}`)).getByText(item.name)).not.toBeNull();
  });
}

async function saveForm() {
  const save = await screen.findByRole("button", { name: "Save changes" });
  await waitFor(() => expect((save as HTMLButtonElement).disabled).toBe(false));
  fireEvent.click(save);
}

async function createThroughSettings() {
  const request = deferred(original);
  transport.create.mockReturnValue(request.promise);
  fireEvent.click(screen.getByRole("button", { name: "Add secret" }));
  fireEvent.change(screen.getByPlaceholderText(NAME_PLACEHOLDER), {
    target: { value: created.name },
  });
  fireEvent.change(screen.getByPlaceholderText("Secret value"), {
    target: { value: "synthetic-test-value" },
  });
  await saveForm();
  await waitFor(() =>
    expect(transport.create).toHaveBeenCalledWith(
      {
        name: created.name,
        value: "synthetic-test-value",
        scope: "workspace",
        workspace_id: WORKSPACE_ID,
      },
      { cache: "no-store", workspaceId: WORKSPACE_ID },
    ),
  );
  expectMetadataRows([original]);
  await acknowledge(request, created);
}

async function renameThroughSettings() {
  const request = deferred(original);
  transport.update.mockReturnValue(request.promise);
  fireEvent.click(screen.getByRole("button", { name: `Edit secret ${original.name}` }));
  fireEvent.change(screen.getByPlaceholderText(NAME_PLACEHOLDER), {
    target: { value: renamed.name },
  });
  await saveForm();
  await waitFor(() =>
    expect(transport.update).toHaveBeenCalledWith(
      original.id,
      { name: renamed.name },
      { cache: "no-store", workspaceId: WORKSPACE_ID },
    ),
  );
  expectMetadataRows([original]);
  await acknowledge(request, renamed);
}

async function deleteThroughSettings() {
  const request = deferred<void>(undefined);
  transport.remove.mockReturnValue(request.promise);
  fireEvent.click(screen.getByRole("button", { name: `Delete secret ${original.name}` }));
  const confirm = await screen.findByTestId("secret-delete-confirm");
  await waitFor(() => expect((confirm as HTMLButtonElement).disabled).toBe(false));
  expect(transport.references).toHaveBeenCalledWith(original.id, {
    cache: "no-store",
    workspaceId: WORKSPACE_ID,
  });
  fireEvent.click(confirm);
  await waitFor(() =>
    expect(transport.remove).toHaveBeenCalledWith(original.id, {
      cache: "no-store",
      workspaceId: WORKSPACE_ID,
    }),
  );
  expectMetadataRows([original]);
  await acknowledge(request, undefined);
}

async function mutateThroughSettings(mutation: Mutation) {
  if (mutation === "add") {
    await createThroughSettings();
    return [original, created];
  }
  if (mutation === "update") {
    await renameThroughSettings();
    return [renamed];
  }
  await deleteThroughSettings();
  return [];
}

function globalSnapshot() {
  return JSON.parse(screen.getByTestId("global-secrets-snapshot").textContent ?? "null");
}

beforeEach(() => {
  Object.values(transport).forEach((mock) => mock.mockReset());
  transport.references.mockResolvedValue([]);
});
afterEach(async () => {
  await act(async () => pending.splice(0).forEach((resolve) => resolve()));
  cleanup();
});

// @covers AC-WORKSPACES-REPOSITORY-SECRETS-001.1
// @covers AC-WORKSPACES-REPOSITORY-SECRETS-001.2
// @covers AC-WORKSPACES-REPOSITORY-SECRETS-001.14
describe("rendered workspace settings acknowledgments", () => {
  const cases = SOURCES.flatMap((source) =>
    MUTATIONS.flatMap((mutation) => OUTCOMES.map((outcome) => ({ source, mutation, outcome }))),
  );
  it.each(cases)(
    "retains acknowledged create rename and delete while a sibling global consumer loads: $source $mutation $outcome",
    async ({ source, mutation, outcome }) => {
      const workspaceRead = deferred<SecretListItem[]>([]);
      const globalRead = deferred<SecretListItem[]>([]);
      transport.list.mockImplementation((options: SecretListOptions) =>
        options.scope === "workspace" ? workspaceRead.promise : globalRead.promise,
      );
      const initialItems = source === "supplied" ? [original] : undefined;
      const view = render(<SettingsTree initialItems={initialItems} />);
      if (source === "fetched") await acknowledge(workspaceRead, [original]);
      expectMetadataRows([original]);

      const expected = await mutateThroughSettings(mutation);
      // Ordinary acknowledged mutation control before the Global consumer mounts.
      expectMetadataRows(expected);
      const scopedRequests = () =>
        transport.list.mock.calls.filter(
          ([options]) => (options as SecretListOptions).scope === "workspace",
        );
      expect(scopedRequests()).toHaveLength(source === "fetched" ? 1 : 0);
      view.rerender(<SettingsTree initialItems={initialItems} globalEnabled />);
      expect(globalSnapshot()).toEqual({ items: [], loaded: false, loading: true });
      expectMetadataRows(expected);

      if (outcome === "success") await acknowledge(globalRead, [globalItem]);
      else await act(async () => globalRead.reject(new Error("global list unavailable")));
      expect(globalSnapshot()).toEqual({
        items: outcome === "success" ? [globalItem.id] : [],
        loaded: true,
        loading: false,
      });
      expectMetadataRows(expected);
      expect(scopedRequests()).toHaveLength(source === "fetched" ? 1 : 0);
    },
  );
});
