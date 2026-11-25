import { act, fireEvent, render, waitFor } from "@testing-library/react";
import { afterEach, expect, it } from "vitest";
import { defaultFeatureFlags } from "@/lib/state/slices/features/types";
import { StateProvider, useAppStore, useAppStoreApi } from "@/components/state-provider";
import {
  SettingsSaveProvider,
  useSettingsSaveCoordinator,
} from "@/components/settings/settings-save-provider";
import SSHExecutorPage from "@/app/settings/executors/ssh/[executorId]/page";
import { RemoteDockerConnectionSection } from "@/components/settings/remote-docker-connection-section";
import {
  createDestination,
  Destinations,
  disposeFixtures,
  executor,
  form,
  json,
  managedTransport,
  OTHER,
  OWNER,
  profile,
  publish,
  transport,
} from "./executor-connection-catalogue-publication.test-helpers";

const NEW_DESTINATION = "destination-new-owner-profile";
const DOCKERFILE = "FROM alpine";
const replacementConnection = "replacement";

afterEach(disposeFixtures);

function RemoteConnection() {
  const target = useAppStore((state) => state.executors.items.find((item) => item.id === OWNER));
  return target ? <RemoteDockerConnectionSection executor={target} /> : null;
}
function mountCaller(fixture: ReturnType<typeof transport>, remote = false) {
  const capture = {} as {
    store: ReturnType<typeof useAppStoreApi>;
    coordinator: ReturnType<typeof useSettingsSaveCoordinator>;
  };
  function Observe() {
    capture.store = useAppStoreApi();
    capture.coordinator = useSettingsSaveCoordinator();
    return <Destinations />;
  }
  const view = render(
    <StateProvider
      initialState={{
        executors: { items: fixture.seed },
        features: { ...defaultFeatureFlags, sshConnectionManager: true },
      }}
    >
      <SettingsSaveProvider>
        {remote ? <RemoteConnection /> : <SSHExecutorPage executorId={OWNER} />}
        <Observe />
      </SettingsSaveProvider>
    </StateProvider>,
  );
  return { view, capture };
}
async function editAndTest(view: ReturnType<typeof render>, remote = false) {
  if (!remote) {
    const name = await view.findByLabelText("Name", { exact: true });
    const picker = view.getByLabelText("Shared connection");
    await waitFor(() =>
      expect(picker.querySelector(`option[value="${replacementConnection}"]`)).toBeTruthy(),
    );
    fireEvent.change(name, { target: { value: form.name } });
    fireEvent.change(picker, { target: { value: replacementConnection } });
    const test = view.getByRole("button", { name: "Test executor", exact: true });
    await waitFor(() => expect(test.hasAttribute("disabled")).toBe(false));
    fireEvent.click(test);
    await waitFor(() =>
      expect(view.getByRole("button", { name: "Save changes" }).hasAttribute("disabled")).toBe(
        false,
      ),
    );
    return;
  }
  await waitFor(() => expect(view.getByTestId("ssh-input-name")).toBeTruthy());
  fireEvent.change(view.getByTestId("ssh-input-name"), { target: { value: form.name } });
  fireEvent.click(view.getAllByTestId("ssh-test-button")[0]);
  await waitFor(() => expect(view.getAllByTestId("ssh-trust-checkbox")[0]).toBeTruthy());
  fireEvent.click(view.getAllByTestId("ssh-trust-checkbox")[0]);
  await waitFor(() =>
    expect(view.getByRole("button", { name: "Save changes" }).getAttribute("disabled")).toBeNull(),
  );
}
async function startCoordinated(
  fixture: ReturnType<typeof transport>,
  capture: ReturnType<typeof mountCaller>["capture"],
) {
  let completion!: ReturnType<typeof capture.coordinator.saveAll>;
  await act(async () => {
    completion = capture.coordinator.saveAll();
    fixture.completions.push(completion);
    await fixture.patchAdmitted.promise;
    fixture.patch.resolve(json(executor()));
    await fixture.listAdmitted.promise;
  });
  return { completion };
}

// @covers AC-EXECUTORS-PROFILE-EDITOR-001.21 through .24
it("SSH page shows the selected shared connection after coordinated save", async () => {
  const fixture = managedTransport();
  const { view, capture } = mountCaller(fixture);
  await editAndTest(view);
  const { completion } = await startCoordinated(fixture, capture);
  act(() => {
    createDestination(capture.store);
    publish(capture.store, "executor.deleted", { id: OTHER });
    publish(capture.store, "executor.profile.updated", {
      ...profile(`${OWNER}-profile`),
      name: "Current destination",
    });
  });
  expect(view.getByTestId(NEW_DESTINATION)).toBeTruthy();
  await act(async () => {
    fixture.list.resolve(json({ executors: fixture.fresh }));
    expect((await completion).canLeave).toBe(true);
  });
  await waitFor(() =>
    expect((view.getByLabelText("Name", { exact: true }) as HTMLInputElement).value).toBe(
      "Normalized host",
    ),
  );
  await waitFor(() =>
    expect((view.getByLabelText("Shared connection") as HTMLSelectElement).value).toBe(
      replacementConnection,
    ),
  );
  expect(fixture.payload).toEqual({
    name: form.name,
    config: {
      ssh_connection_id: replacementConnection,
      ssh_connection_stamp: replacementConnection,
      ssh_shell: "bash",
    },
  });
  expect(view.getByTestId(NEW_DESTINATION).textContent).toContain("new-owner");
  expect(view.getByTestId(`destination-${OWNER}-profile`).textContent).toContain(
    "Current destination",
  );
  expect(view.getByTestId(`destination-${OWNER}-profile`).textContent).toContain("Normalized host");
  expect(view.queryByTestId(`destination-${OTHER}-profile`)).toBeNull();
  expect(fixture.reads).toBe(2);
  expect(fixture.calls.filter((call) => call === "POST /api/v1/ssh/test")).toHaveLength(1);
});

it("Remote Docker section retains config and normalized pin", async () => {
  const remote = {
    ...executor(OWNER, "remote_docker"),
    config: { ...executor().config, dockerfile: DOCKERFILE, mcp_policy: "kept" },
  };
  const unavailable = executor("unavailable", "plugin_remote");
  const fixture = transport([remote, executor(OTHER), unavailable]);
  fixture.fresh[0].config = {
    ...fixture.fresh[0].config,
    dockerfile: DOCKERFILE,
    mcp_policy: "kept",
  };
  const { view, capture } = mountCaller(fixture, true);
  await editAndTest(view, true);
  const { completion } = await startCoordinated(fixture, capture);
  act(() => {
    createDestination(capture.store);
    publish(capture.store, "executor.updated", { id: OTHER, name: "Live sibling" });
    publish(capture.store, "executor.profile.updated", {
      ...profile(`${OTHER}-profile`, OTHER),
      name: "Live profile",
    });
  });
  await act(async () => {
    fixture.list.resolve(json({ executors: fixture.fresh }));
    expect((await completion).canLeave).toBe(true);
  });
  await waitFor(() =>
    expect(view.getByTestId("ssh-fingerprint-pinned-value").textContent).toBe("SHA256:normalized"),
  );
  expect(capture.store.getState().executors.items[0].config).toMatchObject({
    dockerfile: DOCKERFILE,
    mcp_policy: "kept",
  });
  expect(fixture.payload).toMatchObject({
    name: form.name,
    config: {
      dockerfile: DOCKERFILE,
      mcp_policy: "kept",
      ssh_host_fingerprint: "SHA256:submitted",
    },
  });
  expect(view.getByTestId(`destination-${OTHER}-profile`).textContent).toContain("Live sibling");
  expect(view.getByTestId(`destination-${OTHER}-profile`).textContent).toContain("Live profile");
  expect(view.getAllByTestId(NEW_DESTINATION)).toHaveLength(1);
  expect(view.getByTestId("destination-unavailable-profile").getAttribute("data-disabled")).toBe(
    "true",
  );
  expect(fixture.calls.filter((call) => call === "POST /api/v1/remote-docker/test")).toHaveLength(
    1,
  );
  expect(fixture.calls).not.toContain("POST /api/v1/ssh/test");
});

it.each([false, true])(
  "PATCH rejection preserves live state and recoverable rendered draft (remote %s)",
  async (remote) => {
    const fixture = remote
      ? transport([executor(OWNER, "remote_docker"), executor(OTHER)])
      : managedTransport();
    const { view, capture } = mountCaller(fixture, remote);
    await editAndTest(view, remote);
    let completion!: ReturnType<typeof capture.coordinator.saveAll>;
    await act(async () => {
      completion = capture.coordinator.saveAll();
      fixture.completions.push(completion);
      await fixture.patchAdmitted.promise;
    });
    act(() => createDestination(capture.store));
    const live = capture.store.getState().executors.items;
    await act(async () => {
      fixture.patch.reject(new Error("save denied"));
      expect((await completion).canLeave).toBe(false);
    });
    expect(capture.store.getState().executors.items).toBe(live);
    const name = remote
      ? view.getByTestId("ssh-input-name")
      : view.getByLabelText("Name", { exact: true });
    expect((name as HTMLInputElement).value).toBe(form.name);
    expect(name.closest('[data-settings-dirty="true"]')).toBeTruthy();
    expect(capture.coordinator.contributorStates.some((entry) => entry.saveFailed)).toBe(true);
    expect(view.getByTestId(NEW_DESTINATION)).toBeTruthy();
    expect(fixture.calls).not.toContain("GET /api/v1/executors");
    expect(fixture.reads).toBe(remote ? 0 : 1);
  },
);
