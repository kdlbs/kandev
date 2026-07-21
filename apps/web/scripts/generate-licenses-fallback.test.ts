import { describe, expect, it } from "vitest";

import { readGoEntriesFromManifest, recoverGoEntriesAfterReportFailure } from "./generate-licenses";

const APACHE_2 = "Apache-2.0";
const EXISTING_GO_MODULE = "github.com/example/existing";
const MODULE_VERSION = "v1.2.3";
const OLD_MODULE_VERSION = "v1.0.0";

describe("readGoEntriesFromManifest", () => {
  it("returns valid Go entries from an existing manifest", () => {
    const raw = JSON.stringify([
      {
        name: "left-pad",
        version: "1.0.0",
        license: "MIT",
        ecosystem: "npm",
      },
      {
        name: "github.com/example/module",
        version: MODULE_VERSION,
        license: APACHE_2,
        repository: "https://github.com/example/module",
        license_text: "license text",
        ecosystem: "go",
      },
      {
        name: "github.com/example/broken",
        version: MODULE_VERSION,
        ecosystem: "go",
      },
    ]);

    expect(readGoEntriesFromManifest(raw)).toEqual([
      {
        name: "github.com/example/module",
        version: MODULE_VERSION,
        license: APACHE_2,
        repository: "https://github.com/example/module",
        license_text: "license text",
        ecosystem: "go",
      },
    ]);
  });

  it("returns an empty list for invalid or non-array JSON", () => {
    expect(readGoEntriesFromManifest("{")).toEqual([]);
    expect(readGoEntriesFromManifest("{}")).toEqual([]);
    expect(readGoEntriesFromManifest("null")).toEqual([]);
    expect(readGoEntriesFromManifest('""')).toEqual([]);
  });

  it("returns an empty list when the manifest contains only npm entries", () => {
    const raw = JSON.stringify([
      {
        name: "left-pad",
        version: "1.0.0",
        license: "MIT",
        ecosystem: "npm",
      },
    ]);

    expect(readGoEntriesFromManifest(raw)).toEqual([]);
  });

  it("filters malformed Go entries", () => {
    const raw = JSON.stringify([
      {
        name: "github.com/example/missing-license",
        version: "v1.0.0",
        ecosystem: "go",
      },
      {
        version: "v1.0.0",
        license: "MIT",
        ecosystem: "go",
      },
    ]);

    expect(readGoEntriesFromManifest(raw)).toEqual([]);
  });
});

describe("recoverGoEntriesAfterReportFailure", () => {
  it("prefers parseable go-licenses stdout over existing manifest entries", () => {
    const stdout =
      "github.com/example/stdout|||/missing/LICENSE|||https://github.com/example/stdout/blob/v1.2.3/LICENSE|||MIT\n";
    const existingManifest = JSON.stringify([
      {
        name: EXISTING_GO_MODULE,
        version: OLD_MODULE_VERSION,
        license: APACHE_2,
        ecosystem: "go",
      },
    ]);

    expect(recoverGoEntriesAfterReportFailure(stdout, existingManifest)).toEqual({
      source: "go-licenses-stdout",
      entries: [
        {
          name: "github.com/example/stdout",
          version: "v1.2.3",
          license: "MIT",
          repository: "https://github.com/example/stdout",
          ecosystem: "go",
        },
      ],
    });
  });

  it("reuses existing Go entries with a stale marker when stdout has no entries", () => {
    const existingManifest = JSON.stringify([
      {
        name: EXISTING_GO_MODULE,
        version: OLD_MODULE_VERSION,
        license: APACHE_2,
        ecosystem: "go",
      },
    ]);

    expect(recoverGoEntriesAfterReportFailure("", existingManifest)).toEqual({
      source: "existing-manifest",
      entries: [
        {
          name: EXISTING_GO_MODULE,
          version: OLD_MODULE_VERSION,
          license: APACHE_2,
          stale: true,
          ecosystem: "go",
        },
      ],
    });
  });

  it("returns null when neither stdout nor the existing manifest has Go entries", () => {
    expect(recoverGoEntriesAfterReportFailure("", JSON.stringify([]))).toBeNull();
    expect(recoverGoEntriesAfterReportFailure("")).toBeNull();
  });
});
