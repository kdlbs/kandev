import fs from "node:fs";
import path from "node:path";

import { describe, expect, it, vi } from "vitest";

import { planSelfUpdate, runSelfUpdateCommand, type SelfUpdateIntent } from "./self_update";

function intent(kind: "homebrew" | "npm" | "npx" = "npm"): SelfUpdateIntent {
  return {
    version: 1,
    target_tag: "v1.2.3",
    target_version: "1.2.3",
    latest_url: "https://example/v1.2.3",
    created_at: "2026-05-29T00:00:00.000Z",
    install: {
      version: 1,
      manager: "systemd",
      mode: "user",
      kind,
      home_dir: "/home/alice/.kandev",
      log_dir: "/home/alice/.kandev/logs",
      service_path: "/home/alice/.config/systemd/user/kandev.service",
      node_path: "/usr/bin/node",
      cli_entry: "/usr/lib/node_modules/kandev/bin/cli.js",
      port: 38429,
      installed_at: "2026-05-29T00:00:00.000Z",
    },
  };
}

describe("planSelfUpdate", () => {
  it("plans npm upgrade, service reinstall, then user-service restart", () => {
    expect(planSelfUpdate(intent("npm"), { platform: "linux" })).toEqual([
      { command: "npm", args: ["install", "-g", "--prefix", "/usr", "kandev@1.2.3"] },
      {
        command: "/usr/bin/node",
        args: [
          "/usr/lib/node_modules/kandev/bin/cli.js",
          "service",
          "install",
          "--home-dir",
          "/home/alice/.kandev",
          "--port",
          "38429",
        ],
      },
      { command: "systemctl", args: ["--user", "restart", "kandev"] },
    ]);
  });

  it("keeps npm updates inside the original global prefix", () => {
    const npm = intent("npm");
    npm.install.cli_entry = "/tmp/kandev-test/npm-global/lib/node_modules/kandev/bin/cli.js";

    expect(planSelfUpdate(npm, { platform: "linux" })[0]).toEqual({
      command: "npm",
      args: ["install", "-g", "--prefix", "/tmp/kandev-test/npm-global", "kandev@1.2.3"],
    });
  });

  it("falls back to npm's configured prefix when the cli path is non-standard", () => {
    const npm = intent("npm");
    npm.install.cli_entry = "/opt/kandev/bin/cli.js";

    expect(planSelfUpdate(npm, { platform: "linux" })[0]).toEqual({
      command: "npm",
      args: ["install", "-g", "kandev@1.2.3"],
    });
  });

  it("plans npx reinstall without mutating global npm packages", () => {
    const commands = planSelfUpdate(intent("npx"), { platform: "linux" });
    expect(commands[0]).toEqual({
      command: "npx",
      args: [
        "-y",
        "kandev@1.2.3",
        "service",
        "install",
        "--home-dir",
        "/home/alice/.kandev",
        "--port",
        "38429",
      ],
    });
  });

  it("plans launchd restart on macOS", () => {
    const mac = intent("homebrew");
    mac.install.manager = "launchd";
    mac.install.service_path = "/Users/alice/Library/LaunchAgents/com.kdlbs.kandev.plist";
    mac.install.home_dir = "/Users/alice/.kandev";
    mac.install.log_dir = "/Users/alice/.kandev/logs";

    const commands = planSelfUpdate(mac, { platform: "darwin", uid: 501 });
    expect(commands.at(-1)).toEqual({
      command: "launchctl",
      args: ["kickstart", "-k", "gui/501/com.kdlbs.kandev"],
    });
  });
});

describe("runSelfUpdateCommand", () => {
  it("does not run commands in dry-run mode", () => {
    const tmp = fs.mkdtempSync(path.join(process.cwd(), "self-update-test-"));
    const intentPath = path.join(tmp, "intent.json");
    fs.writeFileSync(intentPath, JSON.stringify(intent("npm")));
    const runner = vi.fn();
    const log = vi.spyOn(console, "log").mockImplementation(() => undefined);
    try {
      runSelfUpdateCommand({ action: "self-update", intent: intentPath, dryRun: true }, runner);
      expect(runner).not.toHaveBeenCalled();
      expect(log).toHaveBeenCalledWith(expect.stringContaining('"dry_run": true'));
    } finally {
      log.mockRestore();
      fs.rmSync(tmp, { recursive: true, force: true });
    }
  });

  function readLog(logDir: string): string {
    const file = fs.readdirSync(logDir).find((name) => name.startsWith("self-update-"));
    expect(file, "a self-update log file should be written").toBeTruthy();
    return fs.readFileSync(path.join(logDir, file as string), "utf8");
  }

  it("tees each command, its output, and exit status to a log file", () => {
    const tmp = fs.mkdtempSync(path.join(process.cwd(), "self-update-test-"));
    const logDir = path.join(tmp, "logs");
    const it_ = intent("npm");
    it_.install.log_dir = logDir;
    const intentPath = path.join(tmp, "intent.json");
    fs.writeFileSync(intentPath, JSON.stringify(it_));
    const stdout = vi.spyOn(process.stdout, "write").mockImplementation(() => true);
    const runner = vi.fn().mockReturnValue({ status: 0, stdout: Buffer.from("ok output") });
    try {
      runSelfUpdateCommand({ action: "self-update", intent: intentPath }, runner);
      const contents = readLog(logDir);
      expect(contents).toContain("npm install -g");
      expect(contents).toContain("service install");
      expect(contents).toContain("ok output");
      expect(contents).toContain("exit 0");
      expect(contents).toContain("self-update completed successfully");
    } finally {
      stdout.mockRestore();
      fs.rmSync(tmp, { recursive: true, force: true });
    }
  });

  it("logs the failing command and its code before throwing", () => {
    const tmp = fs.mkdtempSync(path.join(process.cwd(), "self-update-test-"));
    const logDir = path.join(tmp, "logs");
    const it_ = intent("npm");
    it_.install.log_dir = logDir;
    const intentPath = path.join(tmp, "intent.json");
    fs.writeFileSync(intentPath, JSON.stringify(it_));
    const stderr = vi.spyOn(process.stderr, "write").mockImplementation(() => true);
    const stdout = vi.spyOn(process.stdout, "write").mockImplementation(() => true);
    const runner = vi.fn().mockReturnValue({ status: 7, stderr: Buffer.from("npm boom") });
    try {
      expect(() =>
        runSelfUpdateCommand({ action: "self-update", intent: intentPath }, runner),
      ).toThrow(/failed with code 7/);
      const contents = readLog(logDir);
      expect(contents).toContain("npm boom");
      expect(contents).toContain("exited with code 7");
      expect(contents).not.toContain("self-update completed successfully");
    } finally {
      stdout.mockRestore();
      stderr.mockRestore();
      fs.rmSync(tmp, { recursive: true, force: true });
    }
  });
});
