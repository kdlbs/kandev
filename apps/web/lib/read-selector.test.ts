import { describe, expect, it } from "vitest";
import { splitReadFiles, type ReadFileRef } from "./read-selector";

const ref = (path: string, startLine = 0, lineCount = 0): ReadFileRef => ({
  path,
  startLine,
  lineCount,
});

describe("splitReadFiles", () => {
  // Single-file inputs must behave exactly like the Go `Split`: one entry with
  // the selector stripped and the range parsed.
  it.each([
    ["no selector", "config.json", [ref("config.json")]],
    ["single line", "apps/web/lib/utils.ts:50", [ref("apps/web/lib/utils.ts", 50)]],
    ["open-ended range", "main.go:50-", [ref("main.go", 50)]],
    [
      "closed range",
      "apps/backend/internal/sentry/handlers.go:43-94",
      [ref("apps/backend/internal/sentry/handlers.go", 43, 52)],
    ],
    ["plus span", "main.go:50+150", [ref("main.go", 50, 150)]],
    ["multi-range stays one file", "main.go:5-16,960-973", [ref("main.go", 5, 12)]],
    ["raw mode", "main.go:raw", [ref("main.go")]],
    ["range then raw combo", "main.go:2-4:raw", [ref("main.go", 2, 3)]],
    [
      "windows absolute",
      "C:\\Users\\me\\handlers.go:43-94",
      [ref("C:\\Users\\me\\handlers.go", 43, 52)],
    ],
    ["no extension single line", "Makefile:10", [ref("Makefile", 10)]],
  ])("keeps %s as a single file", (_name, raw, want) => {
    expect(splitReadFiles(raw as string)).toEqual(want);
  });

  it("splits two comma-joined files each with its own range", () => {
    expect(
      splitReadFiles(
        "deployments/cluster-tools/values.backupprod.yaml:1-80,deployments/cluster-tools/values.au-backupprod.yaml:1-80",
      ),
    ).toEqual([
      ref("deployments/cluster-tools/values.backupprod.yaml", 1, 80),
      ref("deployments/cluster-tools/values.au-backupprod.yaml", 1, 80),
    ]);
  });

  it("splits two bare-extension files", () => {
    expect(splitReadFiles("a.go:1,b.go:2")).toEqual([ref("a.go", 1), ref("b.go", 2)]);
  });

  it("attaches a trailing multi-range to the first file, not a new file", () => {
    expect(splitReadFiles("a.go:5-16,40-80,b.go:10")).toEqual([
      ref("a.go", 5, 12),
      ref("b.go", 10),
    ]);
  });

  it("splits files carrying mode selectors", () => {
    expect(splitReadFiles("a.go:2-4:raw,b.go:raw")).toEqual([ref("a.go", 2, 3), ref("b.go")]);
  });

  it("keeps a second file openable even without a range", () => {
    expect(splitReadFiles("a/x.yaml:1-80,b/y.yaml")).toEqual([
      ref("a/x.yaml", 1, 80),
      ref("b/y.yaml"),
    ]);
  });

  it("treats a comma inside a directory name as a single file", () => {
    expect(splitReadFiles("a,b/foo.go:1-80")).toEqual([ref("a,b/foo.go", 1, 80)]);
  });

  it("splits the hyphenated multi-file example each with its own range", () => {
    // The hyphens in "tailscale-ingress-extras" must not be mistaken for a line
    // range separator when deciding a segment is a new file.
    expect(
      splitReadFiles(
        "deployments/tailscale-ingress-extras/values.prod.yaml:1-180,deployments/tailscale-ingress-extras/values.staging.yaml:1-180",
      ),
    ).toEqual([
      ref("deployments/tailscale-ingress-extras/values.prod.yaml", 1, 180),
      ref("deployments/tailscale-ingress-extras/values.staging.yaml", 1, 180),
    ]);
  });

  it("splits a legacy half-stripped multi-file path (trailing file's range moved to offset)", () => {
    // Pre-fix backend stored the bundle with only the LAST file's range stripped;
    // both files must still split into openable links.
    expect(
      splitReadFiles(
        "deployments/tailscale-ingress-extras/values.prod.yaml:1-180,deployments/tailscale-ingress-extras/values.staging.yaml",
      ),
    ).toEqual([
      ref("deployments/tailscale-ingress-extras/values.prod.yaml", 1, 180),
      ref("deployments/tailscale-ingress-extras/values.staging.yaml"),
    ]);
  });

  it("falls back to a single entry when a segment is not file-like", () => {
    // "5" alone is a line spec, not a file, so this is not a real multi-file read.
    expect(splitReadFiles("main.go:1,5")).toEqual([ref("main.go", 1)]);
  });

  it("treats a comma inside a filename as a single file", () => {
    // "src/foo" is file-like only via its separator (no selector, no extension),
    // so it is not a file boundary and the whole path stays one entry.
    expect(splitReadFiles("src/foo,bar.go:1-20")).toEqual([ref("src/foo,bar.go", 1, 20)]);
  });

  it("still splits two files when the first carries an extension", () => {
    expect(splitReadFiles("src/a.go,src/b.go:1-20")).toEqual([
      ref("src/a.go"),
      ref("src/b.go", 1, 20),
    ]);
  });

  it("still splits an extensionless first file that carries a selector", () => {
    expect(splitReadFiles("Makefile:10,foo.go:20")).toEqual([
      ref("Makefile", 10),
      ref("foo.go", 20),
    ]);
  });
});
