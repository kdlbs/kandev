const returnLabel = "settings:sshManager.binding.returnToExecutor";
const manageLabel = "settings:sshManager.binding.manage";
const unsavedName = "Unsaved name";
const testLabel = "settings:sshManager.binding.test";
const nameLabel = "settings:sshManager.label";
const readyLabel = "settings:sshManager.binding.executorReady";
import { defaultFeatureFlags } from "@/lib/state/slices/features/types";
import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
import { StateProvider } from "@/components/state-provider";
import { usePathname } from "@/lib/routing/client-router";
import {
  clearNavigationBlockerForTests,
  setNavigationBlocker,
} from "@/lib/routing/navigation-guard";
import type { SSHRoute } from "@/lib/types/http-ssh-connections";
import { SettingsSaveProvider, useSettingsSaveContributor } from "../settings-save-provider";
import { ManagedBinding } from "./managed-binding";
import { SetupReturnActions } from "./setup-return-actions";
import { SSHSetupContinuationProvider, useSSHSetupContinuation } from "./setup-continuation";
vi.mock("react-i18next", () => ({ useTranslation: () => ({ t: (key: string) => key }) }));
const fixture = vi.hoisted(() => ({
  unrelated: false,
  name: "Original",
  save: vi.fn(),
  test: vi.fn(),
}));
vi.mock("@/lib/api/domains/ssh-api", () => ({
  testSSHConnection: (...args: unknown[]) => fixture.test(...args),
}));
const routes: SSHRoute[] = ["first", "second"].map((id) => ({
  destination: {
    id,
    name: id,
    host: "fixture.invalid",
    user: "build",
    port: 22,
    identity_id: "key",
    fingerprint: "pin",
    public_key: "public",
    version: 1,
    revision: 1,
    verified_route: id,
  },
  stamp: id,
}));
vi.mock("@/hooks/domains/settings/use-ssh-connections", () => ({
  useSSHConnections: () => ({ routes, identities: [], loaded: true, busy: false, error: null }),
}));
const editor = "/settings/executors/new/ssh";
const existingEditor = "/settings/executors/ssh/existing";
afterEach(() => {
  cleanup();
  clearNavigationBlockerForTests();
});
function View({ existing }: { existing: boolean }) {
  const pathname = usePathname();
  const origin = existing ? existingEditor : editor;
  return (
    <SettingsSaveProvider key={pathname}>
      <OtherDraft />
      {pathname === origin ? (
        <ManagedBinding
          initial={{ name: fixture.name, connection_id: "first", connection_stamp: "first" }}
          coordinatedSaveId={existing ? "binding" : undefined}
          onSave={fixture.save}
        />
      ) : (
        <SetupReturnActions route={routes[1]} busy={false} />
      )}
    </SettingsSaveProvider>
  );
}
function setup(existing = false, unrelated = false) {
  fixture.unrelated = unrelated;
  fixture.name = "Original";
  fixture.save.mockReset();
  fixture.test.mockReset();
  fixture.test.mockResolvedValue({ success: true, git_version: "git version 2.44.0", steps: [] });
  window.history.replaceState({}, "", existing ? existingEditor : editor);
  return render(
    <StateProvider
      initialState={{ features: { ...defaultFeatureFlags, sshConnectionManager: true } }}
    >
      <SSHSetupContinuationProvider>
        <DraftProbe />
        <View existing={existing} />
      </SSHSetupContinuationProvider>
    </StateProvider>,
  );
}
const click = (name: string) => fireEvent.click(screen.getByRole("button", { name }));
function changeName() {
  fireEvent.change(screen.getByLabelText(nameLabel), { target: { value: unsavedName } });
}
// @covers AC-EXECUTORS-SSH-CONNECTIONS-004.7
it.each([false, true])("preserves the draft without saving (existing=%s)", async (existing) => {
  setup(existing);
  changeName();
  click(manageLabel);
  await waitFor(() => expect(screen.getByRole("button", { name: returnLabel })).toBeTruthy());
  click(returnLabel);
  expect((screen.getByLabelText(nameLabel) as HTMLInputElement).value).toBe(unsavedName);
  expect(fixture.save).not.toHaveBeenCalled();
});
it("requires a new executor check after an explicit connection selection", async () => {
  setup();
  click(testLabel);
  await screen.findByText(readyLabel);
  click(manageLabel);
  click("settings:sshManager.binding.useConnection");
  expect(
    (screen.getByLabelText("settings:sshManager.binding.connection") as HTMLSelectElement).value,
  ).toBe("second");
  expect(screen.getByRole("button", { name: "common:save" }).hasAttribute("disabled")).toBe(true);
  expect(fixture.save).not.toHaveBeenCalled();
});
it("blocks a returned draft when the server baseline changed", () => {
  setup();
  changeName();
  click(manageLabel);
  fixture.name = "Changed by another admin";
  click(returnLabel);
  expect(screen.getByRole("alert").textContent).toContain(
    "settings:sshManager.binding.draftConflict",
  );
  expect((screen.getByLabelText(nameLabel) as HTMLInputElement).value).toBe(unsavedName);
  click("settings:sshManager.binding.reloadDraft");
  expect((screen.getByLabelText(nameLabel) as HTMLInputElement).value).toBe(
    "Changed by another admin",
  );
});

function OtherDraft() {
  useSettingsSaveContributor({
    id: "other",
    revision: 1,
    isDirty: fixture.unrelated,
    save: async () => {},
    discard: () => {},
  });
  return null;
}
it("does not transfer an unrelated dirty contributor when managing connections", () => {
  setup(true, true);
  changeName();
  click(manageLabel);
  expect(window.location.pathname).toBe(existingEditor);
  expect((screen.getByLabelText(nameLabel) as HTMLInputElement).value).toBe(unsavedName);
  expect(screen.queryByRole("button", { name: returnLabel })).toBeNull();
  expect(fixture.save).not.toHaveBeenCalled();
});
it("hides Use connection when the destination readiness is stale", () => {
  const original = routes[1].destination.verified_route;
  routes[1].destination.verified_route = "old";
  try {
    setup();
    click(manageLabel);
    expect(
      screen.queryByRole("button", { name: "settings:sshManager.binding.useConnection" }),
    ).toBeNull();
    click(returnLabel);
    expect(
      (screen.getByLabelText("settings:sshManager.binding.connection") as HTMLSelectElement).value,
    ).toBe("first");
  } finally {
    routes[1].destination.verified_route = original;
  }
});

function DraftProbe() {
  const flow = useSSHSetupContinuation();
  return <output data-testid="setup-draft">{JSON.stringify(flow.draft)}</output>;
}
it("restores the editor when a navigation guard rejects the setup detour", () => {
  setup();
  changeName();
  setNavigationBlocker((intent) => intent.cancel());
  click(manageLabel);
  expect(window.location.pathname).toBe(editor);
  expect((screen.getByLabelText(nameLabel) as HTMLInputElement).value).toBe(unsavedName);
  expect(screen.getByTestId("setup-draft").textContent).toBe("null");
  expect(fixture.save).not.toHaveBeenCalled();
});
it("clears the returned continuation after a successful explicit save", async () => {
  setup();
  click(manageLabel);
  click(returnLabel);
  expect(screen.getByTestId("setup-draft").textContent).not.toBe("null");
  click(testLabel);
  await screen.findByText(readyLabel);
  click("common:save");
  await waitFor(() => expect(screen.getByTestId("setup-draft").textContent).toBe("null"));
  expect(fixture.save).toHaveBeenCalledTimes(1);
});

it("reports the failed platform check, then shows a successful retry with unconfirmed Git", async () => {
  setup();
  fixture.test
    .mockResolvedValueOnce({
      success: false,
      steps: [{ name: "Verify platform", success: false, error: "raw server output" }],
    })
    .mockResolvedValueOnce({ success: true, steps: [], agentctl_action: "skipped" });

  click(testLabel);
  const failure = await screen.findByRole("alert");
  expect(failure.textContent).toContain("settings:sshManager.binding.testPlatformFailed");
  expect(failure.textContent).not.toContain("raw server output");
  const save = screen.getByRole("button", { name: "common:save" });
  expect(save.hasAttribute("disabled")).toBe(true);

  click(testLabel);
  const success = await screen.findByText(readyLabel);
  expect(success.closest('[role="status"]')?.textContent).toContain(
    "settings:sshManager.binding.testGitUnconfirmed",
  );
  expect(success.closest('[role="status"]')?.textContent).toContain(
    "settings:sshManager.binding.testHelperUnconfirmed",
  );
  expect(screen.queryByRole("alert")).toBeNull();
  expect(save.hasAttribute("disabled")).toBe(false);
});

it("opens Access for the existing imported binding without changing its selection", () => {
  const route = routes[0];
  const original = route.destination.verified_route;
  route.destination.verified_route = undefined;
  try {
    setup(true);
    click("settings:sshManager.binding.review");
    const query = new URLSearchParams(window.location.search);
    expect(query.get("id")).toBe("first");
    expect(query.get("section")).toBe("access");
    expect(fixture.save).not.toHaveBeenCalled();
  } finally {
    route.destination.verified_route = original;
  }
});

it.each([false, true])(
  "lets an existing complete import run a diagnostic test (deferred bastion trust=%s)",
  async (hasBastion) => {
    const target = routes[0].destination;
    const original = target.verified_route;
    target.verified_route = undefined;
    target.imported_pending = true;
    routes[0].bastion = hasBastion
      ? { ...target, id: "bastion", fingerprint: "", known_hosts_pending: true }
      : undefined;
    try {
      setup(true);
      expect(screen.getByText("settings:sshManager.binding.imported")).toBeTruthy();
      expect(
        screen.queryByRole("button", { name: "settings:sshManager.binding.review" }),
      ).toBeNull();
      click(testLabel);
      await screen.findByText(readyLabel);
      expect(fixture.test).toHaveBeenCalledWith({
        name: "Original",
        ssh_connection_id: "first",
        expected_stamp: "first",
      });
      expect(fixture.save).not.toHaveBeenCalled();
    } finally {
      target.verified_route = original;
      target.imported_pending = undefined;
      routes[0].bastion = undefined;
    }
  },
);

it("shows the original alias when the imported destination is unresolved", () => {
  const original = routes[0].destination;
  routes[0].destination = {
    ...original,
    host: "",
    alias: "legacy-production",
    repair_reason: "ssh_migration_alias_missing",
    verified_route: undefined,
  };
  try {
    setup(true);
    expect(screen.getByText("legacy-production")).toBeTruthy();
    expect(screen.getByRole("button", { name: testLabel }).hasAttribute("disabled")).toBe(true);
  } finally {
    routes[0].destination = original;
  }
});
