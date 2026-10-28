import fs from "node:fs";
import os from "node:os";
import path from "node:path";
import { describe, expect, it } from "vitest";
import { computeBuildId, shellAssetsFromHtml } from "./service-worker";

describe("shellAssetsFromHtml", () => {
  it("collects the entry script and stylesheets, skipping modulepreload and external URLs", () => {
    const html = `<script type="module" crossorigin src="/assets/index-AbC.js"></script>
      <link rel="stylesheet" crossorigin href="/assets/index-XyZ.css">
      <link rel="modulepreload" href="/assets/vendor-123.js">
      <link rel="icon" href="/icon.svg"><script src="https://cdn.example/x.js"></script>`;
    expect(shellAssetsFromHtml(html)).toEqual(["/assets/index-AbC.js", "/assets/index-XyZ.css"]);
  });
});

const OFFLINE = "/offline.html";

describe("computeBuildId", () => {
  it("changes when a precached file's bytes or the worker source change", () => {
    const dir = fs.mkdtempSync(path.join(os.tmpdir(), "kandev-sw-"));
    try {
      fs.writeFileSync(path.join(dir, "offline.html"), "one");
      const first = computeBuildId(dir, [OFFLINE], "sw-v1");
      expect(computeBuildId(dir, [OFFLINE], "sw-v1")).toBe(first);
      expect(computeBuildId(dir, [OFFLINE], "sw-v2")).not.toBe(first);
      fs.writeFileSync(path.join(dir, "offline.html"), "two");
      expect(computeBuildId(dir, [OFFLINE], "sw-v1")).not.toBe(first);
    } finally {
      fs.rmSync(dir, { recursive: true, force: true });
    }
  });
});
