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

// Versioned bin dir typical of per-user node managers (fnm/nvm/asdf/volta/mise).
const FNM_LAUNCHER: LauncherInfo = {
  nodePath: "/home/alice/.local/share/fnm/node-versions/v24.14.0/installation/bin/node",
  cliEntry:
    "/home/alice/.local/share/fnm/node-versions/v24.14.0/installation/lib/node_modules/kandev/bin/cli.js",
  kind: "npm",
};

// A Homebrew launcher whose floating shim has been resolved. nodePath/cliEntry
// still point at the versioned Cellar paths (as captured at install time), but
// shimPath is the version-independent `<prefix>/bin/kandev` symlink that
// survives `brew upgrade`. See issue #1162.
const SHIM_LAUNCHER: LauncherInfo = {
  nodePath: "/home/linuxbrew/.linuxbrew/Cellar/node/26.0.0/bin/node",
  cliEntry: "/home/linuxbrew/.linuxbrew/Cellar/kandev/0.52.0/libexec/cli/bin/cli.js",
  kind: "homebrew",
  bundleDir: "/home/linuxbrew/.linuxbrew/Cellar/kandev/0.52.0/libexec",
  version: "0.52.0",
  shimPath: "/home/linuxbrew/.linuxbrew/bin/kandev",
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

  it("prepends %h/.local/bin to PATH for user-mode units", () => {
    const unit = renderSystemdUnit({
      launcher: NPM_LAUNCHER,
      homeDir: "/home/alice/.kandev",
      logDir: "/home/alice/.kandev/logs",
      mode: "user",
    });
    expect(unit).toMatch(
      /^Environment=PATH=%h\/\.local\/bin:%h\/\.bun\/bin:\/usr\/local\/bin:\/usr\/bin:\/bin:\/opt\/homebrew\/bin:\/home\/linuxbrew\/\.linuxbrew\/bin$/m,
    );
  });

  it("includes %h/.bun/bin in PATH for user-mode units so Bun-global agent CLIs (e.g. omp) resolve", () => {
    const unit = renderSystemdUnit({
      launcher: NPM_LAUNCHER,
      homeDir: "/home/alice/.kandev",
      logDir: "/home/alice/.kandev/logs",
      mode: "user",
    });
    expect(unit).toContain("%h/.bun/bin");
  });

  it("omits %h/.local/bin from PATH for system-mode units", () => {
    const unit = renderSystemdUnit({
      launcher: NPM_LAUNCHER,
      homeDir: "/var/lib/kandev",
      logDir: "/var/lib/kandev/logs",
      mode: "system",
      systemUser: "alice",
    });
    expect(unit).not.toContain("%h/.local/bin");
    expect(unit).toMatch(/^Environment=PATH=\/usr\/local\/bin/m);
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

  it("prepends launcher node bin dir to PATH so npx-based agents resolve under fnm/nvm/asdf/volta/mise", () => {
    const unit = renderSystemdUnit({
      launcher: FNM_LAUNCHER,
      homeDir: "/home/alice/.kandev",
      logDir: "/home/alice/.kandev/logs",
      mode: "user",
    });
    expect(unit).toContain(
      "Environment=PATH=/home/alice/.local/share/fnm/node-versions/v24.14.0/installation/bin:%h/.local/bin:%h/.bun/bin:/usr/local/bin:/usr/bin:/bin:/opt/homebrew/bin:/home/linuxbrew/.linuxbrew/bin",
    );
  });

  it("does not duplicate node bin dir when it is already on the system PATH", () => {
    const unit = renderSystemdUnit({
      launcher: NPM_LAUNCHER,
      homeDir: "/home/alice/.kandev",
      logDir: "/home/alice/.kandev/logs",
      mode: "user",
    });
    // /usr/local/bin already in SYSTEMD_USER_PATH — dirname(nodePath) must not double it.
    expect(unit).toContain(
      "Environment=PATH=%h/.local/bin:%h/.bun/bin:/usr/local/bin:/usr/bin:/bin:/opt/homebrew/bin:/home/linuxbrew/.linuxbrew/bin",
    );
    expect(unit).not.toContain("/usr/local/bin:/usr/local/bin");
  });

  it("prepends node bin dir for system-mode units too", () => {
    const unit = renderSystemdUnit({
      launcher: FNM_LAUNCHER,
      homeDir: "/var/lib/kandev",
      logDir: "/var/lib/kandev/logs",
      mode: "system",
      systemUser: "alice",
    });
    expect(unit).toContain(
      "Environment=PATH=/home/alice/.local/share/fnm/node-versions/v24.14.0/installation/bin:/usr/local/bin:/usr/bin:/bin:/opt/homebrew/bin:/home/linuxbrew/.linuxbrew/bin",
    );
  });

  it("does not prepend '.' when nodePath has no POSIX separator (Windows-style or bare filename)", () => {
    // path.dirname('C:\\...\\node.exe') === '.' on POSIX. Without the
    // isAbsolute guard, this would put CWD first in the daemon's PATH —
    // a privilege-escalation footgun the systemd unit must not introduce.
    const unit = renderSystemdUnit({
      launcher: {
        nodePath: 'C:\\Program Files\\node "Node"\\node.exe',
        cliEntry: "/home/alice/cli.js",
        kind: "unknown",
      },
      homeDir: "/home/alice/.kandev",
      logDir: "/home/alice/.kandev/logs",
      mode: "user",
    });
    expect(unit).not.toMatch(/^Environment=PATH=\.:/m);
    expect(unit).toContain(
      "Environment=PATH=%h/.local/bin:%h/.bun/bin:/usr/local/bin:/usr/bin:/bin:/opt/homebrew/bin:/home/linuxbrew/.linuxbrew/bin",
    );
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

  // Regression: https://github.com/kdlbs/kandev/issues/1162 — the unit must not
  // bake version-pinned Cellar paths for Homebrew installs, or it crash-loops
  // after `brew upgrade` deletes the old Cellar dir.
  it("uses the floating Homebrew shim in ExecStart when shimPath is set", () => {
    const unit = renderSystemdUnit({
      launcher: SHIM_LAUNCHER,
      homeDir: "/home/alice/.kandev",
      logDir: "/home/alice/.kandev/logs",
      mode: "user",
    });
    expect(unit).toContain("ExecStart=/home/linuxbrew/.linuxbrew/bin/kandev --headless");
    // No version-pinned Cellar paths anywhere in the unit — this single check
    // covers both the kandev cli.js path and the versioned node bin dir, since
    // any `/Cellar/node/...` or `/Cellar/kandev/...` path contains "/Cellar/".
    expect(unit).not.toContain("/Cellar/");
  });

  it("drops KANDEV_BUNDLE_DIR / KANDEV_VERSION and the versioned node bin dir when using the shim", () => {
    const unit = renderSystemdUnit({
      launcher: SHIM_LAUNCHER,
      homeDir: "/home/alice/.kandev",
      logDir: "/home/alice/.kandev/logs",
      mode: "user",
    });
    expect(unit).not.toContain("KANDEV_BUNDLE_DIR");
    expect(unit).not.toContain("KANDEV_VERSION");
    // PATH must fall back to the static base path, not the versioned Cellar node
    // bin. The shim's own bin dir (/home/linuxbrew/.linuxbrew/bin) is already in
    // the base path, so it is not duplicated.
    expect(unit).toContain(
      "Environment=PATH=%h/.local/bin:%h/.bun/bin:/usr/local/bin:/usr/bin:/bin:/opt/homebrew/bin:/home/linuxbrew/.linuxbrew/bin",
    );
  });

  it("keeps versioned node + cli.js ExecStart for npm installs (no shim)", () => {
    const unit = renderSystemdUnit({
      launcher: NPM_LAUNCHER,
      homeDir: "/home/alice/.kandev",
      logDir: "/home/alice/.kandev/logs",
      mode: "user",
    });
    expect(unit).toContain(
      "ExecStart=/usr/local/bin/node /usr/local/lib/node_modules/kandev/bin/cli.js --headless",
    );
  });

  it("prepends the shim's bin dir to PATH for a custom Homebrew prefix not in the base PATH", () => {
    const unit = renderSystemdUnit({
      launcher: {
        nodePath: "/home/me/.brew/Cellar/node/26.0.0/bin/node",
        cliEntry: "/home/me/.brew/Cellar/kandev/0.52.0/libexec/cli/bin/cli.js",
        kind: "homebrew",
        shimPath: "/home/me/.brew/bin/kandev",
      },
      homeDir: "/home/me/.kandev",
      logDir: "/home/me/.kandev/logs",
      mode: "user",
    });
    // Custom prefix's bin dir is prepended so npm/npx resolve, even though it
    // isn't one of the hardcoded default prefixes.
    expect(unit).toContain("Environment=PATH=/home/me/.brew/bin:%h/.local/bin:%h/.bun/bin:");
    expect(unit).not.toContain("/Cellar/");
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

  it("prepends launcher node bin dir to PATH for plists too (fnm/nvm/asdf/volta/mise)", () => {
    const plist = renderLaunchdPlist({
      launcher: {
        nodePath: "/Users/alice/.volta/tools/image/node/24.14.0/bin/node",
        cliEntry: "/Users/alice/.volta/tools/image/packages/kandev/bin/cli.js",
        kind: "npm",
      },
      homeDir: "/Users/alice/.kandev",
      logDir: "/Users/alice/.kandev/logs",
      mode: "user",
    });
    expect(plist).toMatch(
      /<key>PATH<\/key>\s*<string>\/Users\/alice\/\.volta\/tools\/image\/node\/24\.14\.0\/bin:[^<]+\/\.local\/bin:[^<]+\/\.bun\/bin:\/opt\/homebrew\/bin:\/usr\/local\/bin:\/usr\/bin:\/bin<\/string>/,
    );
  });

  it("does not duplicate node bin dir in plist PATH when already present", () => {
    const plist = renderLaunchdPlist({
      launcher: BREW_LAUNCHER,
      homeDir: "/Users/alice/.kandev",
      logDir: "/Users/alice/.kandev/logs",
      mode: "user",
    });
    // /opt/homebrew/bin already in LAUNCHD_USER_PATH — must not be doubled.
    expect(plist).toMatch(
      /<key>PATH<\/key>\s*<string>[^<]+\/\.local\/bin:[^<]+\/\.bun\/bin:\/opt\/homebrew\/bin:\/usr\/local\/bin:\/usr\/bin:\/bin<\/string>/,
    );
    expect(plist).not.toContain("/opt/homebrew/bin:/opt/homebrew/bin");
  });

  it("prepends $HOME/.local/bin to PATH for user-mode plists", () => {
    const plist = renderLaunchdPlist({
      launcher: NPM_LAUNCHER,
      homeDir: "/Users/alice/.kandev",
      logDir: "/Users/alice/.kandev/logs",
      mode: "user",
    });
    expect(plist).toMatch(
      /<key>PATH<\/key>\s*<string>[^<]+\/\.local\/bin:[^<]+\/\.bun\/bin:\/opt\/homebrew\/bin:\/usr\/local\/bin:\/usr\/bin:\/bin<\/string>/,
    );
  });

  it("includes $HOME/.bun/bin in PATH for user-mode plists so Bun-global agent CLIs (e.g. omp) resolve", () => {
    const plist = renderLaunchdPlist({
      launcher: NPM_LAUNCHER,
      homeDir: "/Users/alice/.kandev",
      logDir: "/Users/alice/.kandev/logs",
      mode: "user",
    });
    expect(plist).toMatch(/<key>PATH<\/key>\s*<string>[^<]*\/\.bun\/bin:/);
  });

  it("prepends node bin dir for system-mode LaunchDaemons too", () => {
    const plist = renderLaunchdPlist({
      launcher: {
        nodePath: "/Users/alice/.volta/tools/image/node/24.14.0/bin/node",
        cliEntry: "/Users/alice/.volta/tools/image/packages/kandev/bin/cli.js",
        kind: "npm",
      },
      homeDir: "/Library/Application Support/kandev",
      logDir: "/Library/Logs/kandev",
      mode: "system",
      systemUser: "_kandev",
    });
    expect(plist).toContain(
      "<key>PATH</key>\n      <string>/Users/alice/.volta/tools/image/node/24.14.0/bin:/opt/homebrew/bin:/usr/local/bin:/usr/bin:/bin</string>",
    );
  });

  it("omits $HOME/.local/bin from PATH for system-mode plists", () => {
    const plist = renderLaunchdPlist({
      launcher: NPM_LAUNCHER,
      homeDir: "/Library/Application Support/kandev",
      logDir: "/Library/Logs/kandev",
      mode: "system",
      systemUser: "_kandev",
    });
    expect(plist).not.toContain("/.local/bin");
    expect(plist).toContain(
      "<key>PATH</key>\n      <string>/opt/homebrew/bin:/usr/local/bin:/usr/bin:/bin</string>",
    );
  });

  it("does not prepend '.' to plist PATH when nodePath has no POSIX separator", () => {
    const plist = renderLaunchdPlist({
      launcher: {
        nodePath: "C:\\Program Files\\node\\node.exe",
        cliEntry: "/home/alice/cli.js",
        kind: "unknown",
      },
      homeDir: "/Users/alice/.kandev",
      logDir: "/Users/alice/.kandev/logs",
      mode: "user",
    });
    expect(plist).not.toMatch(/<key>PATH<\/key>\s*<string>\.:/);
    expect(plist).toMatch(
      /<key>PATH<\/key>\s*<string>[^<]+\/\.local\/bin:[^<]+\/\.bun\/bin:\/opt\/homebrew\/bin:\/usr\/local\/bin:\/usr\/bin:\/bin<\/string>/,
    );
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

  it("quotes Environment= lines when value contains a space (greptile P1 regression)", () => {
    const unit = renderSystemdUnit({
      launcher: NPM_LAUNCHER,
      homeDir: "/home/john doe/.kandev",
      logDir: "/home/john doe/.kandev/logs",
      mode: "user",
    });
    // The whole assignment must be wrapped, not just the value.
    expect(unit).toContain('Environment="KANDEV_HOME_DIR=/home/john doe/.kandev"');
    // PATH always contains colons but no spaces — should NOT be quoted.
    expect(unit).toMatch(/^Environment=PATH=%h\/\.local\/bin:%h\/\.bun\/bin:\/usr\/local\/bin/m);
  });

  it("escapes backslash + double-quote in Environment= and ExecStart values", () => {
    const unit = renderSystemdUnit({
      launcher: {
        nodePath: 'C:\\Program Files\\node "Node"\\node.exe',
        cliEntry: "/home/alice/cli.js",
        kind: "unknown",
      },
      homeDir: "C:\\Program Files\\kandev",
      logDir: "/var/log",
      mode: "user",
    });
    // Backslashes doubled, quotes escaped, whole thing wrapped.
    expect(unit).toContain('Environment="KANDEV_HOME_DIR=C:\\\\Program Files\\\\kandev"');
    expect(unit).toContain(
      'ExecStart="C:\\\\Program Files\\\\node \\"Node\\"\\\\node.exe" /home/alice/cli.js --headless',
    );
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

  // Regression: https://github.com/kdlbs/kandev/issues/1162
  it("uses the floating Homebrew shim in ProgramArguments when shimPath is set", () => {
    const plist = renderLaunchdPlist({
      launcher: SHIM_LAUNCHER,
      homeDir: "/Users/alice/.kandev",
      logDir: "/Users/alice/.kandev/logs",
      mode: "user",
    });
    expect(plist).toContain("<string>/home/linuxbrew/.linuxbrew/bin/kandev</string>");
    expect(plist).toContain("<string>--headless</string>");
    expect(plist).not.toContain("/Cellar/");
    expect(plist).not.toContain("KANDEV_BUNDLE_DIR");
    expect(plist).not.toContain("KANDEV_VERSION");
  });
});
