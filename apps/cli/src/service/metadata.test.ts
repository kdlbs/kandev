import fs from "node:fs";
import path from "node:path";

import { describe, expect, it } from "vitest";

import {
  buildServiceInstallMetadata,
  serviceMetadataPath,
  writeServiceInstallMetadata,
} from "./metadata";

describe("service metadata", () => {
  it("builds the install metadata written beside service units", () => {
    const metadata = buildServiceInstallMetadata({
      manager: "systemd",
      mode: "user",
      launcher: {
        nodePath: "/usr/bin/node",
        cliEntry: "/usr/lib/node_modules/kandev/bin/cli.js",
        kind: "npm",
      },
      homeDir: "/home/alice/.kandev",
      logDir: "/home/alice/.kandev/logs",
      servicePath: "/home/alice/.config/systemd/user/kandev.service",
      port: 38429,
      now: new Date("2026-05-29T00:00:00.000Z"),
    });

    expect(metadata).toMatchObject({
      version: 1,
      manager: "systemd",
      mode: "user",
      kind: "npm",
      home_dir: "/home/alice/.kandev",
      service_path: "/home/alice/.config/systemd/user/kandev.service",
      node_path: "/usr/bin/node",
      cli_entry: "/usr/lib/node_modules/kandev/bin/cli.js",
      port: 38429,
      installed_at: "2026-05-29T00:00:00.000Z",
    });
  });

  it("writes metadata JSON under <home>/service/install.json", () => {
    const tmp = fs.mkdtempSync(path.join(process.cwd(), "metadata-test-"));
    try {
      const target = serviceMetadataPath(tmp);
      writeServiceInstallMetadata(target, {
        version: 1,
        manager: "launchd",
        mode: "user",
        kind: "homebrew",
        home_dir: tmp,
        log_dir: path.join(tmp, "logs"),
        service_path: "/Users/alice/Library/LaunchAgents/com.kdlbs.kandev.plist",
        node_path: "/opt/homebrew/bin/node",
        cli_entry: "/opt/homebrew/opt/kandev/libexec/cli/bin/cli.js",
        installed_at: "2026-05-29T00:00:00.000Z",
      });

      const parsed = JSON.parse(fs.readFileSync(target, "utf8")) as { manager: string };
      expect(parsed.manager).toBe("launchd");
    } finally {
      fs.rmSync(tmp, { recursive: true, force: true });
    }
  });
});
