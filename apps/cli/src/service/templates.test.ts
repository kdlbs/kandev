import { describe, expect, it } from "vitest";

import type { LauncherInfo } from "./paths";
import { renderLaunchdPlist, renderSystemdUnit } from "./templates";

const NPM_LAUNCHER: LauncherInfo = {
  nodePath: "/usr/local/bin/node",
  cliEntry: "/usr/local/lib/node_modules/kandev/bin/cli.js",
  kind: "npm",
};

const BREW_LAUNCHER: LauncherInfo = {
  nodePath: "/opt/homebrew/bin/node",
  cliEntry: "/opt/homebrew/opt/kandev/libexec/cli/bin/cli.js",
  kind: "homebrew",
  bundleDir: "/opt/homebrew/opt/kandev/libexec",
  version: "0.49.0",
};

describe("renderSystemdUnit", () => {
  it("renders a user unit with absolute paths and --headless", () => {
    const unit = renderSystemdUnit({
      launcher: NPM_LAUNCHER,
      homeDir: "/home/alice/.kandev",
      logDir: "/home/alice/.kandev/logs",
      mode: "user",
    });
    expect(unit).toContain(
      "ExecStart=/usr/local/bin/node /usr/local/lib/node_modules/kandev/bin/cli.js --headless",
    );
    expect(unit).toContain("Environment=KANDEV_HOME_DIR=/home/alice/.kandev");
    expect(unit).toContain("WantedBy=default.target");
    expect(unit).not.toContain("User=");
    expect(unit).not.toContain("KANDEV_BUNDLE_DIR");
    expect(unit).not.toContain("KANDEV_SERVER_PORT");
  });

  it("sets WantedBy=multi-user.target and User= for system mode", () => {
    const unit = renderSystemdUnit({
      launcher: NPM_LAUNCHER,
      homeDir: "/var/lib/kandev",
      logDir: "/var/lib/kandev/logs",
      mode: "system",
      systemUser: "alice",
    });
    expect(unit).toContain("WantedBy=multi-user.target");
    expect(unit).toContain("User=alice");
  });

  it("bakes in Homebrew env vars when present on launcher", () => {
    const unit = renderSystemdUnit({
      launcher: BREW_LAUNCHER,
      homeDir: "/home/alice/.kandev",
      logDir: "/home/alice/.kandev/logs",
      mode: "user",
    });
    expect(unit).toContain("Environment=KANDEV_BUNDLE_DIR=/opt/homebrew/opt/kandev/libexec");
    expect(unit).toContain("Environment=KANDEV_VERSION=0.49.0");
  });

  it("bakes in KANDEV_SERVER_PORT when port is set", () => {
    const unit = renderSystemdUnit({
      launcher: NPM_LAUNCHER,
      homeDir: "/home/alice/.kandev",
      logDir: "/home/alice/.kandev/logs",
      mode: "user",
      port: 9000,
    });
    expect(unit).toContain("Environment=KANDEV_SERVER_PORT=9000");
  });

  it("quotes ExecStart paths that contain spaces", () => {
    const unit = renderSystemdUnit({
      launcher: {
        nodePath: "/Library/Application Support/node",
        cliEntry: "/Library/Application Support/kandev/cli.js",
        kind: "unknown",
      },
      homeDir: "/home/alice/.kandev",
      logDir: "/home/alice/.kandev/logs",
      mode: "user",
    });
    expect(unit).toContain(
      `ExecStart="/Library/Application Support/node" "/Library/Application Support/kandev/cli.js" --headless`,
    );
  });
});

describe("renderLaunchdPlist", () => {
  it("renders a user-agent plist with KeepAlive and --headless", () => {
    const plist = renderLaunchdPlist({
      launcher: NPM_LAUNCHER,
      homeDir: "/Users/alice/.kandev",
      logDir: "/Users/alice/.kandev/logs",
      mode: "user",
    });
    expect(plist).toContain("<string>com.kdlbs.kandev</string>");
    expect(plist).toContain("<string>/usr/local/bin/node</string>");
    expect(plist).toContain("<string>--headless</string>");
    expect(plist).toContain("<key>KeepAlive</key>");
    expect(plist).toContain("<key>RunAtLoad</key>");
    expect(plist).toContain("<string>/Users/alice/.kandev/logs/service.err</string>");
    expect(plist).toContain("KANDEV_HOME_DIR");
    expect(plist).not.toContain("KANDEV_BUNDLE_DIR");
  });

  it("escapes XML special characters in paths", () => {
    const plist = renderLaunchdPlist({
      launcher: {
        nodePath: "/path/with/<&'\">/node",
        cliEntry: "/path/cli.js",
        kind: "unknown",
      },
      homeDir: "/Users/alice/.kandev",
      logDir: "/Users/alice/.kandev/logs",
      mode: "user",
    });
    expect(plist).toContain("/path/with/&lt;&amp;&apos;&quot;&gt;/node");
    expect(plist).not.toContain("<&'\"");
  });

  it("includes Homebrew env vars when present", () => {
    const plist = renderLaunchdPlist({
      launcher: BREW_LAUNCHER,
      homeDir: "/Users/alice/.kandev",
      logDir: "/Users/alice/.kandev/logs",
      mode: "user",
    });
    expect(plist).toContain("KANDEV_BUNDLE_DIR");
    expect(plist).toContain("/opt/homebrew/opt/kandev/libexec");
    expect(plist).toContain("KANDEV_VERSION");
  });

  it("emits UserName for system mode with systemUser", () => {
    const plist = renderLaunchdPlist({
      launcher: NPM_LAUNCHER,
      homeDir: "/var/lib/kandev",
      logDir: "/var/lib/kandev/logs",
      mode: "system",
      systemUser: "alice",
    });
    expect(plist).toContain("<key>UserName</key>");
    expect(plist).toContain("<string>alice</string>");
  });

  it("omits UserName for user mode", () => {
    const plist = renderLaunchdPlist({
      launcher: NPM_LAUNCHER,
      homeDir: "/Users/alice/.kandev",
      logDir: "/Users/alice/.kandev/logs",
      mode: "user",
    });
    expect(plist).not.toContain("<key>UserName</key>");
  });
});
