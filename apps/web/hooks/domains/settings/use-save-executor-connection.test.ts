import { act } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import {
  beginSave,
  createDestination,
  deferred,
  disposeFixtures,
  executor,
  form,
  json,
  mountHook,
  OTHER,
  OWNER,
  profile,
  publish,
  settle,
  submittedConfig,
  transport,
} from "./executor-connection-catalogue-publication.test-helpers";

const NEW_PROFILE = "new-owner-profile";
const DELETED = "executor.deleted";
const UPDATED = "executor.updated";

afterEach(disposeFixtures);

describe("useSaveExecutorConnection current catalogue", () => {
  // @covers AC-EXECUTORS-PROFILE-EDITOR-001.21
  it("successful refresh retains a created destination", async () => {
    const fixture = transport();
    const { result } = mountHook(fixture);
    const { completion } = await beginSave(fixture, () => result.current.save(form));
    act(() => createDestination(result.current.store));
    expect(result.current.options.map((option) => option.value)).toContain(NEW_PROFILE);
    await settle(fixture, completion);
    expect
      .soft(result.current.store.getState().executors.items.map((item) => item.id))
      .toContain("new-owner");
    expect(result.current.options.map((option) => option.value)).toContain(NEW_PROFILE);
  });

  it("successful refresh does not resurrect a deleted sibling", async () => {
    const fixture = transport();
    const { result } = mountHook(fixture);
    const { completion } = await beginSave(fixture, () => result.current.save(form));
    act(() => publish(result.current.store, DELETED, { id: OTHER }));
    expect(result.current.options.map((option) => option.value)).not.toContain(`${OTHER}-profile`);
    await settle(fixture, completion);
    expect
      .soft(result.current.store.getState().executors.items.map((item) => item.id))
      .not.toContain(OTHER);
    expect(result.current.options.map((option) => option.value)).not.toContain(`${OTHER}-profile`);
  });

  // @covers AC-EXECUTORS-PROFILE-EDITOR-001.23
  it("failed refresh retains live membership", async () => {
    const fixture = transport();
    const { result } = mountHook(fixture);
    const { completion } = await beginSave(fixture, () => result.current.save(form));
    act(() => {
      createDestination(result.current.store);
      publish(result.current.store, DELETED, { id: OTHER });
    });
    await settle(fixture, completion, true);
    expect(result.current.store.getState().executors.items.map((item) => item.id)).toEqual([
      "connection-owner",
      "new-owner",
    ]);
    expect(result.current.options.map((option) => option.value)).toEqual([
      "connection-owner-profile",
      NEW_PROFILE,
    ]);
  });
});

// @covers AC-EXECUTORS-PROFILE-EDITOR-001.21 through .24
it("mixed executor and profile events survive refresh", async () => {
  const fixture = transport();
  const { result } = mountHook(fixture);
  const { completion } = await beginSave(fixture, () => result.current.save(form));
  act(() => {
    createDestination(result.current.store);
    publish(result.current.store, UPDATED, {
      id: OTHER,
      name: "Current sibling",
      status: "inactive",
      config: { live: "other" },
    });
    publish(result.current.store, "executor.profile.created", profile("sibling-new", OTHER));
    publish(result.current.store, "executor.profile.updated", {
      ...profile(`${OTHER}-profile`, OTHER),
      name: "Current profile",
    });
    publish(result.current.store, "executor.profile.deleted", { id: `${OWNER}-profile` });
    publish(result.current.store, "executor.profile.created", profile("owner-new"));
    createDestination(result.current.store, "gone");
    publish(result.current.store, DELETED, { id: "gone" });
  });
  const live = result.current.store.getState().executors.items;
  await settle(fixture, completion);
  const items = result.current.store.getState().executors.items;
  expect(items.filter((item) => item.id !== OWNER)).toEqual(
    live.filter((item) => item.id !== OWNER),
  );
  expect(items[0].profiles).toEqual(live[0].profiles);
  expect(result.current.options.map((option) => option.value)).toEqual([
    "owner-new",
    `${OTHER}-profile`,
    "sibling-new",
    NEW_PROFILE,
  ]);
  expect(
    result.current.options.find((option) => option.value === `${OTHER}-profile`),
  ).toMatchObject({
    label: "Current profile",
    executorName: "Current sibling",
    executorType: "ssh",
    disabled: false,
  });
});

it("unchanged target adopts normalized name and fingerprint", async () => {
  const fixture = transport();
  const onSaved = vi.fn();
  const { result } = mountHook(fixture, onSaved);
  const { completion } = await beginSave(fixture, () => result.current.save(form));
  expect(fixture.payload).toEqual({ name: form.name, config: submittedConfig() });
  await settle(fixture, completion);
  expect(result.current.store.getState().executors.items[0]).toMatchObject({
    name: "Normalized host",
    config: { ssh_host_fingerprint: "SHA256:normalized" },
  });
  expect(result.current.store.getState().executors.items[0].config).not.toHaveProperty("clear");
  expect(onSaved).toHaveBeenCalledTimes(1);
});

it("profile-only updates allow connection normalization", async () => {
  const fixture = transport();
  const { result } = mountHook(fixture);
  const { completion } = await beginSave(fixture, () => result.current.save(form));
  act(() =>
    publish(result.current.store, "executor.profile.updated", {
      ...profile(`${OWNER}-profile`),
      name: "New profile name",
    }),
  );
  await settle(fixture, completion);
  expect(result.current.store.getState().executors.items[0].profiles?.[0].name).toBe(
    "New profile name",
  );
  expect(result.current.store.getState().executors.items[0].config?.ssh_host_fingerprint).toBe(
    "SHA256:normalized",
  );
});

it.each([false, true])(
  "target field transitions survive PATCH and refresh (fallback %s)",
  async (fail) => {
    const fixture = transport();
    const { result } = mountHook(fixture);
    fixture.fresh[0].config = { ...fixture.fresh[0].config, clear: "stale" };
    const completion = result.current.save(form);
    fixture.completions.push(completion);
    await fixture.patchAdmitted.promise;
    act(() =>
      publish(result.current.store, UPDATED, {
        id: OWNER,
        name: "During PATCH",
        config: { ...executor().config, ssh_host: "live.lan", added: "live" },
      }),
    );
    await act(async () => {
      fixture.patch.resolve(json(executor()));
      await fixture.listAdmitted.promise;
    });
    act(() => {
      publish(result.current.store, UPDATED, {
        id: OWNER,
        name: executor().name,
        status: "inactive",
        updated_at: "later",
        config: {
          ssh_host: "restored.lan",
          ssh_identity_source: "agent",
          added: "live",
          ssh_host_fingerprint: "SHA256:old",
        },
      });
      publish(result.current.store, UPDATED, {
        id: OWNER,
        config: {
          ssh_host: "old.lan",
          ssh_identity_source: "agent",
          added: "live",
          ssh_host_fingerprint: "SHA256:old",
        },
      });
    });
    await settle(fixture, completion, fail);
    expect(result.current.store.getState().executors.items[0]).toMatchObject({
      name: OWNER,
      status: "inactive",
      updated_at: "later",
      config: {
        ssh_host: "old.lan",
        added: "live",
        ssh_host_fingerprint: fail ? "SHA256:submitted" : "SHA256:normalized",
      },
    });
    expect(result.current.store.getState().executors.items[0].config).not.toHaveProperty("clear");
  },
);

it("own-save echo retains delivered normalized values", async () => {
  const fixture = transport();
  const { result } = mountHook(fixture);
  const completion = result.current.save(form);
  fixture.completions.push(completion);
  await fixture.patchAdmitted.promise;
  const echo = {
    id: OWNER,
    name: "Delivered save",
    config: { ...submittedConfig(), ssh_host_fingerprint: "SHA256:echo" },
  };
  act(() => publish(result.current.store, UPDATED, echo));
  await act(async () => {
    fixture.patch.resolve(json(echo));
    await fixture.listAdmitted.promise;
  });
  await settle(fixture, completion);
  expect(result.current.store.getState().executors.items[0]).toMatchObject(echo);
});

it.each(
  ["missing", "deleted", "reintroduced", "empty"].flatMap((state) =>
    [false, true].map((fail) => ({ state, fail })),
  ),
)(
  "removed targets are never restored or patched after reintroduction ($state, fallback $fail)",
  async ({ state, fail }) => {
    const fixture = transport(state === "missing" ? [executor(OTHER)] : undefined);
    fixture.fresh = [executor(), executor(OTHER)];
    const { result } = mountHook(fixture);
    const { completion } = await beginSave(fixture, () => result.current.save(form));
    act(() => {
      if (state !== "missing") publish(result.current.store, DELETED, { id: OWNER });
      if (state === "reintroduced") createDestination(result.current.store, OWNER);
      if (state === "empty") publish(result.current.store, DELETED, { id: OTHER });
    });
    const live = result.current.store.getState().executors.items;
    await settle(fixture, completion, fail);
    expect(result.current.store.getState().executors.items).toEqual(live);
  },
);

it.each(["missing-response", "rejected-refresh"])(
  "missing target and rejected refresh use submitted fallback (%s)",
  async (mode) => {
    const fixture = transport();
    const { result } = mountHook(fixture);
    const { completion } = await beginSave(fixture, () => result.current.save(form));
    act(() =>
      publish(result.current.store, UPDATED, {
        id: OWNER,
        config: { ...executor().config, retained: "live" },
      }),
    );
    fixture.fresh = [executor(OTHER)];
    await settle(fixture, completion, mode === "rejected-refresh");
    expect(result.current.store.getState().executors.items[0].config).toEqual({
      ...submittedConfig(),
      retained: "live",
    });
  },
);

it("PATCH rejection preserves live state and skips callback", async () => {
  const fixture = transport();
  const onSaved = vi.fn();
  const { result } = mountHook(fixture, onSaved);
  const completion = result.current.save(form);
  fixture.completions.push(completion);
  const rejected = expect(completion).rejects.toThrow("save denied");
  await fixture.patchAdmitted.promise;
  act(() => createDestination(result.current.store));
  const live = result.current.store.getState().executors.items;
  await act(async () => {
    fixture.patch.reject(new Error("save denied"));
    await rejected;
  });
  expect(result.current.store.getState().executors.items).toBe(live);
  expect(onSaved).not.toHaveBeenCalled();
  expect(fixture.calls).not.toContain("GET /api/v1/executors");
});

it("onSaved is awaited once after publication", async () => {
  const fixture = transport();
  const callback = deferred<void>();
  const admitted = deferred<void>();
  const onSaved = vi.fn(() => {
    admitted.resolve();
    return callback.promise;
  });
  const { result } = mountHook(fixture, onSaved);
  const { completion } = await beginSave(fixture, () => result.current.save(form));
  let done = false;
  const observed = completion.then(() => {
    done = true;
  });
  await act(async () => {
    fixture.list.resolve(json({ executors: fixture.fresh }));
    await admitted.promise;
  });
  expect(result.current.store.getState().executors.items[0].name).toBe("Normalized host");
  expect(done).toBe(false);
  await act(async () => {
    callback.resolve();
    await observed;
  });
  expect(onSaved).toHaveBeenCalledTimes(1);
});

it("callback rejection propagates without fallback or another callback", async () => {
  const fixture = transport();
  const onSaved = vi.fn(async () => {
    throw new Error("callback denied");
  });
  const { result } = mountHook(fixture, onSaved);
  const { completion } = await beginSave(fixture, () => result.current.save(form));
  const rejected = expect(completion).rejects.toThrow("callback denied");
  await act(async () => {
    fixture.list.resolve(json({ executors: fixture.fresh }));
    await rejected;
  });
  expect(result.current.store.getState().executors.items[0].name).toBe("Normalized host");
  expect(onSaved).toHaveBeenCalledTimes(1);
});

it.each(["success", "patch-failure", "refresh-failure", "callback-failure", "unmounted"])(
  "save observation is isolated and released (%s)",
  async (mode) => {
    const fixture = transport();
    const onSaved =
      mode === "callback-failure"
        ? async () => {
            throw new Error("callback failed");
          }
        : undefined;
    const hook = mountHook(fixture, onSaved);
    const isolated = mountHook(fixture);
    const store = hook.result.current.store;
    const otherBefore = isolated.result.current.store.getState().executors.items;
    const original = store.subscribe;
    const releases: { disposed: boolean }[] = [];
    vi.spyOn(store, "subscribe").mockImplementation((listener) => {
      const entry = { disposed: false };
      releases.push(entry);
      const unsubscribe = original(listener);
      return () => {
        entry.disposed = true;
        unsubscribe();
      };
    });
    const completion = hook.result.current.save(form);
    fixture.completions.push(completion);
    const outcome = completion.then(
      () => undefined,
      (error: Error) => error.message,
    );
    await fixture.patchAdmitted.promise;
    expect(releases[0]?.disposed).toBe(false);
    if (mode === "unmounted") hook.unmount();
    await act(async () => {
      if (mode === "patch-failure") fixture.patch.reject(new Error("patch failed"));
      else {
        fixture.patch.resolve(json(executor()));
        await fixture.listAdmitted.promise;
      }
      if (mode === "refresh-failure") fixture.list.reject(new Error("list failed"));
      else fixture.list.resolve(json({ executors: fixture.fresh }));
      await outcome;
    });
    expect(releases[0].disposed).toBe(true);
    expect(isolated.result.current.store.getState().executors.items).toBe(otherBefore);
    hook.unmount();
    expect(releases.every((entry) => entry.disposed)).toBe(true);
  },
);
