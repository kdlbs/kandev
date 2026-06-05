import path from "node:path";

import { describe, expect, it } from "vitest";

import {
  KANDEV_HOME_DIR,
  resolveCacheDir,
  resolveDataDir,
  resolveDatabasePath,
  resolveKandevHomeDir,
} from "./constants";

describe("runtime data paths", () => {
  it("defaults to the user's kandev home", () => {
    expect(resolveKandevHomeDir({})).toBe(KANDEV_HOME_DIR);
  });

  it("honors KANDEV_HOME_DIR for service-managed launches", () => {
    const env = { KANDEV_HOME_DIR: "/srv/kandev-test" };

    expect(resolveKandevHomeDir(env)).toBe("/srv/kandev-test");
    expect(resolveCacheDir(env)).toBe(path.join("/srv/kandev-test", "bin"));
    expect(resolveDataDir(env)).toBe(path.join("/srv/kandev-test", "data"));
    expect(resolveDatabasePath(env)).toBe(path.join("/srv/kandev-test", "data", "kandev.db"));
  });

  it("lets an explicit KANDEV_DATABASE_PATH override the home-derived path", () => {
    expect(
      resolveDatabasePath({
        KANDEV_HOME_DIR: "/srv/kandev-test",
        KANDEV_DATABASE_PATH: "/tmp/custom.db",
      }),
    ).toBe("/tmp/custom.db");
  });
});
