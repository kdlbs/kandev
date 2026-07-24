import { expect, test } from "vitest";
import { BackendFixtureEnvOverrides, createScopedEnvUse } from "../e2e/fixtures/backend-env";

test("keeps a scoped backend environment across restarts and restores baseline on release", () => {
  const overrides = new BackendFixtureEnvOverrides();
  const release = overrides.add({
    GIT_CONFIG_COUNT: "1",
    GIT_CONFIG_KEY_0: "url.http://172.17.0.1:40123/.insteadOf",
    GIT_CONFIG_VALUE_0: "https://gitlab.com/",
  });

  const expectedEnv = {
    PATH: "/bin",
    GIT_CONFIG_COUNT: "1",
    GIT_CONFIG_KEY_0: "url.http://172.17.0.1:40123/.insteadOf",
    GIT_CONFIG_VALUE_0: "https://gitlab.com/",
  };
  expect(overrides.apply({ PATH: "/bin" })).toEqual(expectedEnv);
  expect(overrides.apply({ PATH: "/bin" })).toEqual(expectedEnv);

  expect(() =>
    overrides.add({ GIT_CONFIG_KEY_0: "url.http://172.17.0.1:40124/.insteadOf" }),
  ).toThrow("conflicting backend fixture environment override");

  release();
  expect(overrides.apply({ PATH: "/bin" })).toEqual({ PATH: "/bin" });
});

test("merges one-shot restart values without overriding an active scoped value", () => {
  const overrides = new BackendFixtureEnvOverrides();
  const release = overrides.add({ GIT_CONFIG_COUNT: "1" });

  expect(overrides.apply({ PATH: "/bin" }, { KANDEV_MOCK_AGENT: "only" })).toEqual({
    PATH: "/bin",
    GIT_CONFIG_COUNT: "1",
    KANDEV_MOCK_AGENT: "only",
  });
  expect(overrides.apply({ PATH: "/bin" }, { GIT_CONFIG_COUNT: "1" })).toEqual({
    PATH: "/bin",
    GIT_CONFIG_COUNT: "1",
  });
  expect(() => overrides.apply({ PATH: "/bin" }, { GIT_CONFIG_COUNT: "2" })).toThrow(
    "conflicting backend fixture environment override",
  );

  release();
});

test("restores the baseline environment after activation and restoration both fail", async () => {
  const overrides = new BackendFixtureEnvOverrides();
  const activationError = new Error("activation failed");
  const restoreError = new Error("baseline restoration failed");
  const outcomes = [activationError, restoreError];
  const observedEnvironments: Record<string, string>[] = [];
  const useEnv = createScopedEnvUse(overrides, async () => {
    observedEnvironments.push(overrides.apply({ PATH: "/bin" }));
    throw outcomes.shift();
  });

  await expect(useEnv({ GIT_CONFIG_COUNT: "1" })).rejects.toMatchObject({
    errors: [activationError, restoreError],
    message: "failed to activate backend fixture environment and restore baseline",
  });
  expect(observedEnvironments).toEqual([{ PATH: "/bin", GIT_CONFIG_COUNT: "1" }, { PATH: "/bin" }]);
  expect(overrides.apply({ PATH: "/bin" })).toEqual({ PATH: "/bin" });
});

test("retries release until the baseline restart is healthy", async () => {
  const overrides = new BackendFixtureEnvOverrides();
  const releaseError = new Error("baseline restart failed");
  const outcomes = [undefined, releaseError, undefined];
  const observedEnvironments: Record<string, string>[] = [];
  const useEnv = createScopedEnvUse(overrides, async () => {
    observedEnvironments.push(overrides.apply({ PATH: "/bin" }));
    const outcome = outcomes.shift();
    if (outcome) throw outcome;
  });

  const release = await useEnv({ GIT_CONFIG_COUNT: "1" });
  await expect(release()).rejects.toThrow("baseline restart failed");
  expect(overrides.apply({ PATH: "/bin" })).toEqual({ PATH: "/bin" });

  await release();
  await release();
  expect(observedEnvironments).toEqual([
    { PATH: "/bin", GIT_CONFIG_COUNT: "1" },
    { PATH: "/bin" },
    { PATH: "/bin" },
  ]);
});
