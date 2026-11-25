const createIdentityLabel = "settings:sshManager.createIdentityForConnection";
const targetHost = "example.invalid";
import { act, cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
import { ManagerDetail } from "./manager-detail";
import { ConnectionDetails } from "./connection-details";
import { SettingsSaveProvider } from "../settings-save-provider";
import type { SSHManager } from "@/hooks/domains/settings/use-ssh-connections";
import type { SSHRoute } from "@/lib/types/http-ssh-connections";
vi.mock("react-i18next", () => ({ useTranslation: () => ({ t: (key: string) => key }) }));
afterEach(cleanup);
const saved: SSHRoute = {
  destination: {
    id: "target",
    name: "Build",
    host: targetHost,
    user: "build",
    port: 22,
    identity_id: "",
    fingerprint: "",
    public_key: "",
    version: 1,
    revision: 1,
  },
  stamp: "target:1",
};
function setup(saveConnection = vi.fn().mockResolvedValue(saved)) {
  const onCreateIdentity = vi.fn();
  const manager = {
    busy: false,
    identities: [],
    routes: [],
    saveConnection,
  } as unknown as SSHManager;
  const mounted = render(
    <SettingsSaveProvider>
      <ConnectionDetails
        manager={manager}
        onSaved={vi.fn()}
        onDeleted={vi.fn()}
        {...{ onCreateIdentity }}
      />
    </SettingsSaveProvider>,
  );
  fireEvent.change(screen.getByLabelText("settings:sshManager.label"), {
    target: { value: "Build" },
  });
  fireEvent.change(screen.getByLabelText("settings:sshManager.host"), {
    target: { value: targetHost },
  });
  return { ...mounted, onCreateIdentity, saveConnection };
}
// @covers AC-EXECUTORS-SSH-CONNECTIONS-004.7
it("saves connection metadata before opening identity creation", async () => {
  const result = setup();
  fireEvent.click(screen.getByRole("button", { name: createIdentityLabel }));
  await waitFor(() => expect(result.onCreateIdentity).toHaveBeenCalledWith(saved));
  expect(result.saveConnection.mock.calls[0][0].connection.host).toBe(targetHost);
});
it("keeps the entered details when the draft cannot be saved", async () => {
  const result = setup(vi.fn().mockRejectedValue(new Error("conflict")));
  fireEvent.click(screen.getByRole("button", { name: createIdentityLabel }));
  await waitFor(() => expect(result.saveConnection).toHaveBeenCalledTimes(1));
  expect(result.onCreateIdentity).not.toHaveBeenCalled();
  expect((screen.getByLabelText("settings:sshManager.host") as HTMLInputElement).value).toBe(
    targetHost,
  );
});
it("does not open identity creation after the saving form was dismissed", async () => {
  let finish!: (route: SSHRoute) => void;
  const pending = new Promise<SSHRoute>((resolve) => {
    finish = resolve;
  });
  const result = setup(vi.fn().mockReturnValue(pending));
  fireEvent.click(screen.getByRole("button", { name: createIdentityLabel }));
  result.unmount();
  await act(async () => {
    finish(saved);
    await pending;
  });
  expect(result.onCreateIdentity).not.toHaveBeenCalled();
});

it("consumes a returned identity once without saving or overriding later edits", async () => {
  window.history.replaceState(
    {},
    "",
    "/settings/executors/ssh-connections?tab=connections&id=target&select_identity=key",
  );
  const saveConnection = vi.fn();
  const manager = {
    busy: false,
    identities: [{ id: "key", label: "New key" }],
    routes: [],
    saveConnection,
  } as unknown as SSHManager;
  render(
    <SettingsSaveProvider>
      <ConnectionDetails route={saved} manager={manager} onSaved={vi.fn()} onDeleted={vi.fn()} />
    </SettingsSaveProvider>,
  );
  const field = screen.getByLabelText("settings:sshManager.identity") as HTMLSelectElement;
  await waitFor(() => expect(field.value).toBe("key"));
  expect(window.location.search).not.toContain("select_identity");
  expect(document.activeElement).toBe(field);
  expect(saveConnection).not.toHaveBeenCalled();
  fireEvent.change(field, { target: { value: "" } });
  expect(field.value).toBe("");
});
it("reports a missing returned identity without changing the saved selection", async () => {
  window.history.replaceState(
    {},
    "",
    "/settings/executors/ssh-connections?tab=connections&id=target&select_identity=deleted",
  );
  const manager = { busy: false, identities: [], routes: [] } as unknown as SSHManager;
  render(
    <SettingsSaveProvider>
      <ConnectionDetails route={saved} manager={manager} onSaved={vi.fn()} onDeleted={vi.fn()} />
    </SettingsSaveProvider>,
  );
  await screen.findByText("settings:sshManager.recordUnavailable");
  expect((screen.getByLabelText("settings:sshManager.identity") as HTMLSelectElement).value).toBe(
    "",
  );
});

it("requires a name before saving metadata and opening identity creation", () => {
  const result = setup();
  fireEvent.change(screen.getByLabelText("settings:sshManager.label"), { target: { value: "  " } });
  const action = screen.getByRole("button", {
    name: createIdentityLabel,
  });
  expect(action.hasAttribute("disabled")).toBe(true);
  fireEvent.click(action);
  expect(result.saveConnection).not.toHaveBeenCalled();
});
it("does not submit the same draft twice while its save is pending", async () => {
  let finish!: (route: SSHRoute) => void;
  const pending = new Promise<SSHRoute>((resolve) => {
    finish = resolve;
  });
  const result = setup(vi.fn().mockReturnValue(pending));
  const action = screen.getByRole("button", {
    name: createIdentityLabel,
  });
  fireEvent.click(action);
  fireEvent.click(action);
  expect(result.saveConnection).toHaveBeenCalledTimes(1);
  await act(async () => {
    finish(saved);
    await pending;
  });
  expect(result.onCreateIdentity).toHaveBeenCalledTimes(1);
});

// @covers AC-EXECUTORS-SSH-CONNECTIONS-004.3, 004.11
it("starts a new draft when a bookmark contains both a selected record and new=1", () => {
  window.history.replaceState({}, "", "/settings/executors/ssh-connections?id=target&new=1");
  const manager = { busy: false, identities: [], routes: [saved] } as unknown as SSHManager;
  const navigation = {
    identities: false,
    creating: true,
    editing: false,
    selected: "target",
    back: vi.fn(),
    saved: vi.fn(),
    createBastion: vi.fn(),
    createIdentity: vi.fn(),
  } as unknown as import("./use-manager-navigation").ManagerNavigation;
  render(
    <SettingsSaveProvider>
      <ManagerDetail manager={manager} navigation={navigation} />
    </SettingsSaveProvider>,
  );
  expect((screen.getByLabelText("settings:sshManager.host") as HTMLInputElement).value).toBe("");
});
