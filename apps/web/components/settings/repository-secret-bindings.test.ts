import { describe, expect, it } from "vitest";
import { validateRepositorySecretBindings } from "./repository-secret-bindings";

describe("validateRepositorySecretBindings", () => {
  it("accepts POSIX keys with a selected secret", () => {
    expect(
      validateRepositorySecretBindings([{ key: "NPM_TOKEN", secret_id: "secret-1" }]),
    ).toBeNull();
  });

  it.each([
    { key: "", kind: "key" },
    { key: " TOKEN", kind: "key" },
    { key: "TOKEN ", kind: "key" },
    { key: "token-name", kind: "key" },
    { key: "TASK_DESCRIPTION", kind: "reserved" },
    { key: "KANDEV_CUSTOM", kind: "reserved" },
  ])("rejects $key as $kind", ({ key, kind }) => {
    expect(validateRepositorySecretBindings([{ key, secret_id: "secret-1" }])).toMatchObject({
      kind,
    });
  });

  it("enforces the environment key length boundary", () => {
    const maximumKey = "A".repeat(256);
    const tooLongKey = "A".repeat(257);

    expect(
      validateRepositorySecretBindings([{ key: maximumKey, secret_id: "secret-1" }]),
    ).toBeNull();
    expect(
      validateRepositorySecretBindings([{ key: tooLongKey, secret_id: "secret-1" }]),
    ).toMatchObject({ kind: "key" });
  });

  it("rejects duplicate keys and missing references", () => {
    expect(
      validateRepositorySecretBindings([
        { key: "TOKEN", secret_id: "secret-1" },
        { key: "TOKEN", secret_id: "secret-2" },
      ]),
    ).toMatchObject({ kind: "duplicate", key: "TOKEN" });
    expect(validateRepositorySecretBindings([{ key: "TOKEN", secret_id: "" }])).toMatchObject({
      kind: "secret",
      key: "TOKEN",
    });
  });
});
