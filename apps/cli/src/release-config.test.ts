import { readdirSync, readFileSync } from "node:fs";
import { resolve } from "node:path";
import { describe, expect, it } from "vitest";

const repoRoot = resolve(__dirname, "../../..");

function readRepoFile(path: string): string {
  return readFileSync(resolve(repoRoot, path), "utf8");
}

function workflowFiles(): string[] {
  const workflowDir = resolve(repoRoot, ".github/workflows");
  return readdirSync(workflowDir)
    .filter((name) => name.endsWith(".yml") || name.endsWith(".yaml"))
    .map((name) => `.github/workflows/${name}`);
}

function extractDockerPnpmVersion(dockerfile: string): string | undefined {
  return dockerfile.match(/^ARG PNPM_VERSION=([0-9]+\.[0-9]+\.[0-9]+)$/m)?.[1];
}

function extractPackageManagerPnpmVersion(packageJSON: string): string {
  const packageManager = (JSON.parse(packageJSON) as { packageManager?: string }).packageManager;
  const version = packageManager?.match(/^pnpm@([0-9]+\.[0-9]+\.[0-9]+)$/)?.[1];
  expect(version, "apps/package.json: packageManager must pin pnpm").toBeDefined();
  if (version === undefined) {
    throw new Error("apps/package.json: packageManager must pin pnpm");
  }
  return version;
}

function indentation(line: string): number {
  return line.search(/\S/);
}

function isBlankOrComment(line: string): boolean {
  const trimmed = line.trim();
  return trimmed.length === 0 || trimmed.startsWith("#");
}

function parseVersionLine(line: string): string | undefined {
  const match = line.match(/^\s*version:\s*(?:"([^"]+)"|'([^']+)'|([^"'\s#]+))\s*(?:#.*)?$/);
  return match?.[1] ?? match?.[2] ?? match?.[3];
}

function findVersionInWithBlock(
  lines: string[],
  start: number,
  withIndent: number,
): string | undefined {
  for (let index = start; index < lines.length; index += 1) {
    const line = lines[index];
    if (isBlankOrComment(line)) {
      continue;
    }
    if (indentation(line) <= withIndent) {
      return undefined;
    }

    const version = parseVersionLine(line);
    if (version !== undefined) {
      return version;
    }
  }

  return undefined;
}

function findPnpmSetupVersion(
  lines: string[],
  start: number,
  stepIndent: number,
): string | undefined {
  for (let index = start; index < lines.length; index += 1) {
    const line = lines[index];
    if (isBlankOrComment(line)) {
      continue;
    }
    if (indentation(line) <= stepIndent) {
      return undefined;
    }

    const withMatch = line.match(/^(\s*)with:\s*(?:#.*)?$/);
    if (withMatch !== null) {
      return findVersionInWithBlock(lines, index + 1, withMatch[1].length);
    }
  }

  return undefined;
}

function matchPnpmSetupUsesLine(line: string): RegExpMatchArray | null {
  return line.match(/^(\s*)(?:-\s*)?uses:\s*["']?pnpm\/action-setup@v\d+["']?\s*(?:#.*)?$/);
}

function findStepIndent(lines: string[], usesLineIndex: number, usesIndent: number): number {
  if (lines[usesLineIndex].trimStart().startsWith("- ")) {
    return usesIndent;
  }

  for (let index = usesLineIndex - 1; index >= 0; index -= 1) {
    const line = lines[index];
    if (isBlankOrComment(line)) {
      continue;
    }

    const lineIndent = indentation(line);
    if (lineIndent < usesIndent && line.slice(lineIndent).startsWith("- ")) {
      return lineIndent;
    }
  }

  return Math.max(0, usesIndent - 2);
}

function extractWorkflowPnpmVersions(workflow: string): Array<string | undefined> {
  const lines = workflow.split(/\r?\n/);
  const versions: Array<string | undefined> = [];

  for (let index = 0; index < lines.length; index += 1) {
    const line = lines[index];
    const setupMatch = matchPnpmSetupUsesLine(line);
    if (setupMatch !== null) {
      const stepIndent = findStepIndent(lines, index, setupMatch[1].length);
      versions.push(findPnpmSetupVersion(lines, index + 1, stepIndent));
    }
  }

  return versions;
}

function assertWorkflowPnpmVersions(file: string, expectedVersion: string): number {
  const versions = extractWorkflowPnpmVersions(readRepoFile(file));
  for (const version of versions) {
    if (version === undefined) {
      expect(version, `${file}: pnpm/action-setup step is missing a version pin`).toBeDefined();
      continue;
    }

    expect(version, `${file}: pnpm/action-setup version must match apps/package.json`).toBe(
      expectedVersion,
    );
  }

  return versions.length;
}

describe("release package manager version", () => {
  it("pins pnpm consistently for Docker and GitHub Actions", () => {
    const packagePnpmVersion = extractPackageManagerPnpmVersion(readRepoFile("apps/package.json"));
    const dockerfile = readRepoFile("Dockerfile");
    const dockerPnpmVersion = extractDockerPnpmVersion(dockerfile);

    expect(dockerfile).not.toContain("pnpm@latest");
    if (dockerPnpmVersion !== undefined) {
      expect(dockerPnpmVersion, "Dockerfile: PNPM_VERSION must match apps/package.json").toBe(
        packagePnpmVersion,
      );
    }

    const workflowSetupCount = workflowFiles().reduce(
      (count, file) => count + assertWorkflowPnpmVersions(file, packagePnpmVersion),
      0,
    );
    expect(workflowSetupCount).toBeGreaterThan(0);
  });
});

describe("release npm publishing", () => {
  it("skips packages that are already published on npm", () => {
    const script = readRepoFile("scripts/release/publish-npm.sh");

    expect(script).toContain('npm view "${pkg}@${VERSION}" version --silent');
    expect(script).toMatch(
      /if package_already_published "\$pkg"; then\s+record_already_published "\$pkg"\s+continue\s+fi/,
    );
    expect(script).toMatch(
      /if package_already_published "kandev"; then\s+record_already_published "kandev"/,
    );
    expect(script).toContain("EPUBLISHCONFLICT");
    expect(script).toContain("treated as idempotent success");
  });
});

describe("release desktop artifacts", () => {
  const desktopPlatforms = ["macos-arm64", "macos-x64", "linux-x64", "linux-arm64", "windows-x64"];

  function releaseWorkflow(): string {
    return readRepoFile(".github/workflows/release.yml");
  }

  it("builds desktop artifacts for every supported runtime platform", () => {
    const workflow = releaseWorkflow();

    expect(workflow).toMatch(/\n  build-desktop:\n/);
    expect(workflow).toContain("needs: [prepare, build-bundles]");
    expect(workflow).toContain("scripts/release/prepare-desktop-runtime.sh");
    expect(workflow).toContain('--platform "${{ matrix.platform }}"');
    expect(workflow).toContain(
      'rustup toolchain install stable --profile minimal --target "${{ matrix.rust_target }}"',
    );
    expect(workflow).toContain("Swatinem/rust-cache@v2");

    for (const platform of desktopPlatforms) {
      expect(workflow, `release.yml must include desktop platform ${platform}`).toContain(
        `platform: ${platform}`,
      );
    }
  });

  it("publishes desktop artifacts while leaving npm and Homebrew tied to runtime tarballs", () => {
    const workflow = releaseWorkflow();
    const publishNpmScript = readRepoFile("scripts/release/publish-npm.sh");
    const homebrewScript = readRepoFile("scripts/release/update-homebrew-tap.sh");

    expect(workflow).toContain(
      "needs: [prepare, build-bundles, build-desktop, docker-universal-manifest]",
    );
    expect(workflow).toContain("pattern: desktop-*");
    expect(workflow).toContain("scripts/release/verify-desktop-assets.sh");
    expect(workflow).toContain("dist/release-assets/kandev-desktop-*");
    expect(workflow).toContain('shasum -a 256 "$(basename "$dest")"');

    expect(publishNpmScript).toContain('asset="kandev-${platform}.tar.gz"');
    expect(publishNpmScript).not.toContain("kandev-desktop-");
    expect(homebrewScript).toContain('local sha_file="kandev-${platform}.tar.gz.sha256"');
    expect(homebrewScript).not.toContain("kandev-desktop-");
  });

  it("bumps desktop package and Tauri versions during release preparation", () => {
    const workflow = releaseWorkflow();

    expect(workflow).toContain('(cd apps/desktop && npm version --no-git-tag-version "$NEXT")');
    expect(workflow).toContain("tauriConfig.version = next");
    expect(workflow).toContain('cargoToml.replace(/^version = ".*"$/m, `version = "${next}"`)');
    expect(workflow).toContain('name = "kandev-desktop"');
    expect(workflow).toContain("apps/desktop/src-tauri/tauri.conf.json");
    expect(workflow).toContain("apps/desktop/src-tauri/Cargo.toml");
    expect(workflow).toContain("apps/desktop/src-tauri/Cargo.lock");
  });

  it("pins linux x64 desktop release builds to Ubuntu 22.04", () => {
    const workflow = releaseWorkflow();

    expect(workflow).toContain(`- os: ubuntu-22.04
            platform: linux-x64
            goos: linux
            goarch: amd64`);
    expect(workflow).toContain(`- platform: linux-x64
            os: ubuntu-22.04
            rust_target: x86_64-unknown-linux-gnu
            tauri_bundles: deb,rpm`);
  });

  it("supports unsigned desktop releases separately from validation-only builds", () => {
    const workflow = releaseWorkflow();
    const signingDocs = readRepoFile("docs/desktop-tauri-signing.md");
    const tauriConfig = readRepoFile("apps/desktop/src-tauri/tauri.conf.json");
    const windowsSignScript = readRepoFile("apps/desktop/src-tauri/windows-sign.ps1");

    expect(workflow).toContain("allow_unsigned_desktop");
    expect(workflow).toContain("desktop_validation_only");
    expect(workflow).toContain("ref: ${{ needs.prepare.outputs.ref }}");
    expect(workflow).toContain("persist-credentials: false");
    expect(workflow).toContain("Desktop validation summary");
    expect(workflow).toContain("No release PR, tag, GitHub release, public container tags");
    expect(workflow).toContain("if: ${{ !inputs.dry_run && !inputs.desktop_validation_only }}");
    expect(workflow).toContain('if [ "$DESKTOP_VALIDATION_ONLY" = "true" ]; then');
    expect(workflow).toContain(
      "Allow unsigned macOS/Windows desktop artifacts; still publishes unless validation-only is checked",
    );
    expect(workflow).toContain("Add unsigned desktop warning to release notes");
    expect(workflow).toContain(
      "macOS and Windows desktop installers in this release are unsigned.",
    );
    expect(workflow).not.toMatch(
      /if \[ "\$ALLOW_UNSIGNED_DESKTOP" = "true" \]; then\r?\n\s*echo "ref=\$CURRENT_REF"/,
    );
    expect(workflow).toContain("docker-amd64:");
    expect(workflow).toContain("docker-universal-manifest:");
    expect(workflow).toContain('if [ "$ALLOW_UNSIGNED_DESKTOP" = "true" ]; then');
    expect(workflow).toContain("unset APPLE_CERTIFICATE");
    expect(workflow).toContain("unset APPLE_ID");

    for (const key of [
      "APPLE_CERTIFICATE",
      "APPLE_CERTIFICATE_PASSWORD",
      "KEYCHAIN_PASSWORD",
      "APPLE_SIGNING_IDENTITY",
      "APPLE_API_KEY_P8",
      "APPLE_API_KEY_PATH",
      "APPLE_TEAM_ID",
    ]) {
      expect(workflow, `release.yml must wire ${key}`).toContain(key);
    }

    for (const key of ["WINDOWS_CERTIFICATE", "WINDOWS_CERTIFICATE_PASSWORD", "signtool sign"]) {
      expect(workflow, `release.yml must wire ${key}`).toContain(key);
    }
    expect(workflow).toContain("Invoke-SignTool");
    expect(workflow).toContain("$LASTEXITCODE -ne 0");
    expect(workflow).toContain("Sign macOS desktop runtime binaries");
    expect(workflow).toContain("resources/kandev/bin");
    expect(workflow).toContain("for binary in kandev agentctl; do");
    expect(workflow).toContain(
      'codesign --force --options runtime --timestamp --sign "$APPLE_SIGNING_IDENTITY"',
    );
    expect(workflow).toContain("codesign --verify --strict");
    expect(workflow).toContain("Sign Windows desktop runtime binaries");
    expect(workflow).toContain('foreach ($binary in @("kandev.exe", "agentctl.exe"))');
    expect(workflow).toContain("Remove-Item -LiteralPath $certificatePath");

    expect(tauriConfig).toContain('"publisher": "Kandev"');
    expect(tauriConfig).toContain('"timestampUrl"');
    expect(tauriConfig).toContain('"timestampUrl": "https://timestamp.digicert.com"');
    expect(tauriConfig).toContain('"signCommand"');
    expect(tauriConfig).toContain("windows-sign.ps1");
    expect(tauriConfig).not.toContain('"csp": null');
    expect(windowsSignScript).toContain('"https://timestamp.digicert.com"');
    expect(windowsSignScript).toContain("Skipping Windows signing for unsigned desktop artifact");
    expect(windowsSignScript).toContain("Remove-Item -LiteralPath $certificatePath");
    expect(signingDocs).toContain("Public recommended desktop releases require signing");
    expect(signingDocs).toContain("allow_unsigned_desktop");
    expect(signingDocs).toContain("desktop_validation_only");
    expect(signingDocs).toContain("publishes the normal release outputs");
    expect(signingDocs).toContain("does not publish a GitHub release");
    expect(signingDocs).toContain("public container tags");
    expect(signingDocs).toContain("Ubuntu 22.04");
  });
});
