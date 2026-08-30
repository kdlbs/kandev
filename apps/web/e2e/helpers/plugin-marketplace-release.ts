import { execFile, execFileSync } from "node:child_process";
import { createHash } from "node:crypto";
import fs from "node:fs";
import http from "node:http";
import os from "node:os";
import path from "node:path";
import { promisify } from "node:util";

const REPO_ROOT = path.resolve(__dirname, "../../../..");
const BACKEND_ROOT = path.join(REPO_ROOT, "apps/backend");
const REGISTRY_ROOT = path.join(REPO_ROOT, "plugin-registry");
const PLUGIN_ID = "kandev-plugin-e2e";
const REPOSITORY = "acme/kandev-plugin-e2e";
const INITIAL_VERSION = "1.0.0";
const execFileAsync = promisify(execFile);

export class PluginMarketplaceReleaseFixture {
  private readonly tempDir = fs.mkdtempSync(path.join(os.tmpdir(), "kandev-marketplace-release-"));
  private readonly server = http.createServer((request, response) =>
    this.handle(request, response),
  );
  private readonly packages = new Map<string, string>();
  private currentRelease = INITIAL_VERSION;
  private baseUrl = "";

  private constructor() {}

  static async start(): Promise<PluginMarketplaceReleaseFixture> {
    const fixture = new PluginMarketplaceReleaseFixture();
    try {
      fixture.preparePackages();
      await new Promise<void>((resolve, reject) => {
        fixture.server.once("error", reject);
        fixture.server.listen(0, "127.0.0.1", () => resolve());
      });
      const address = fixture.server.address();
      if (!address || typeof address === "string")
        throw new Error("release fixture address unavailable");
      fixture.baseUrl = `http://127.0.0.1:${address.port}`;
      fixture.writeInitialIndex();
      return fixture;
    } catch (error) {
      await fixture.close();
      throw error;
    }
  }

  get indexUrl(): string {
    return `${this.baseUrl}/plugins/index.json`;
  }

  async detectRelease(): Promise<boolean> {
    const outputPath = path.join(this.tempDir, "github-output.txt");
    fs.writeFileSync(outputPath, "");
    await this.runNode("check-releases.mjs", {
      GITHUB_OUTPUT: outputPath,
      PLUGIN_REGISTRY_INDEX_URL: this.indexUrl,
    });
    return fs.readFileSync(outputPath, "utf8").includes("rebuild=true");
  }

  publishRelease(version: string): void {
    if (!this.packages.has(version)) throw new Error(`fixture package ${version} is unavailable`);
    this.currentRelease = version;
  }

  async rebuildIndex(): Promise<void> {
    const outputPath = path.join(this.tempDir, "next-index.json");
    await this.runNode("build-index.mjs", {
      PLUGIN_PACKAGE_VERIFIER: path.join(BACKEND_ROOT, ".build/plugin-package-verify"),
      PLUGIN_REGISTRY_OUTPUT: outputPath,
      PLUGIN_REGISTRY_PRIOR_INDEX: path.join(this.tempDir, "index.json"),
      PLUGIN_REGISTRY_RAW_BASE: `${this.baseUrl}/raw`,
    });
    fs.copyFileSync(outputPath, path.join(this.tempDir, "index.json"));
  }

  async close(): Promise<void> {
    if (this.server.listening) {
      await new Promise<void>((resolve, reject) =>
        this.server.close((error) => (error ? reject(error) : resolve())),
      );
    }
    fs.rmSync(this.tempDir, { recursive: true, force: true });
  }

  private async runNode(script: string, extraEnv: Record<string, string>): Promise<void> {
    await execFileAsync(process.execPath, [path.join(REGISTRY_ROOT, script)], {
      cwd: REPO_ROOT,
      env: {
        ...process.env,
        ...extraEnv,
        PLUGIN_REGISTRY_GITHUB_API: `${this.baseUrl}/api`,
        PLUGIN_REGISTRY_PLUGINS_YAML: path.join(this.tempDir, "plugins.yaml"),
      },
      maxBuffer: 1 << 20,
    });
  }

  private preparePackages(): void {
    const v1Path = path.join(BACKEND_ROOT, `.build/kandev-plugin-e2e-${INITIAL_VERSION}.tar.gz`);
    if (!fs.existsSync(v1Path)) throw new Error(`missing E2E fixture package: ${v1Path}`);
    this.packages.set(INITIAL_VERSION, v1Path);
    const v2Path = this.repackage(v1Path, "2.0.0");
    this.packages.set("1.5.0", this.corruptCopy(v2Path, "1.5.0"));
    this.packages.set("2.0.0", v2Path);
    fs.writeFileSync(
      path.join(this.tempDir, "plugins.yaml"),
      `plugins:\n  - id: ${PLUGIN_ID}\n    repo: ${REPOSITORY}\n`,
    );
  }

  private repackage(source: string, version: string): string {
    const stage = path.join(this.tempDir, `stage-${version}`);
    fs.mkdirSync(stage);
    execFileSync("tar", ["-xzf", source, "-C", stage]);
    const manifestPath = path.join(stage, "manifest.yaml");
    const manifest = fs.readFileSync(manifestPath, "utf8");
    fs.writeFileSync(
      manifestPath,
      manifest.replace(`version: "${INITIAL_VERSION}"`, `version: "${version}"`),
    );
    const checksumPath = path.join(stage, "checksums.txt");
    fs.rmSync(checksumPath, { force: true });
    const files = walkFiles(stage)
      .filter((file) => file !== "checksums.txt" && file !== "checksums.txt.sig")
      .sort();
    const checksums = files.map((file) => `${sha256(path.join(stage, file))}  ${file}`).join("\n");
    fs.writeFileSync(checksumPath, `${checksums}\n`);
    const output = path.join(this.tempDir, `${PLUGIN_ID}-${version}.tar.gz`);
    execFileSync("tar", ["-czf", output, "-C", stage, "."]);
    return output;
  }

  private corruptCopy(source: string, version: string): string {
    const output = path.join(this.tempDir, `${PLUGIN_ID}-${version}.tar.gz`);
    const bytes = fs.readFileSync(source);
    bytes[Math.floor(bytes.length / 2)] ^= 0xff;
    fs.writeFileSync(output, bytes);
    return output;
  }

  private writeInitialIndex(): void {
    const version = INITIAL_VERSION;
    const document = {
      schema_version: 1,
      generated_at: "2026-08-30T00:00:00Z",
      source: { name: "Release fixture", url: this.indexUrl },
      plugins: [this.record(version)],
    };
    fs.writeFileSync(path.join(this.tempDir, "index.json"), `${JSON.stringify(document)}\n`);
  }

  private record(version: string) {
    const packagePath = this.packages.get(version);
    if (!packagePath) throw new Error(`missing package ${version}`);
    return {
      id: PLUGIN_ID,
      name: "Kandev E2E Fixture Plugin",
      description: "Curated release fixture",
      author: "kandev",
      categories: ["tools"],
      icon_url: null,
      repo_url: `https://github.com/${REPOSITORY}`,
      version,
      min_kandev_version: null,
      package_url: `${this.baseUrl}/assets/${path.basename(packagePath)}`,
      package_sha256: sha256(packagePath),
      stars: 1,
      updated_at: "2026-08-30T00:00:00Z",
    };
  }

  private handle(request: http.IncomingMessage, response: http.ServerResponse): void {
    const requestPath = new URL(request.url || "/", this.baseUrl).pathname;
    if (requestPath === "/plugins/index.json") {
      return sendFile(response, path.join(this.tempDir, "index.json"), "application/json");
    }
    if (requestPath === `/api/repos/${REPOSITORY}/releases/latest`) {
      const packagePath = this.packages.get(this.currentRelease)!;
      return sendJSON(response, {
        tag_name: `v${this.currentRelease}`,
        name: `Release ${this.currentRelease}`,
        published_at: new Date().toISOString(),
        assets: [
          {
            name: path.basename(packagePath),
            browser_download_url: `${this.baseUrl}/assets/${path.basename(packagePath)}`,
          },
          {
            name: "checksums.txt",
            browser_download_url: `${this.baseUrl}/assets/checksums.txt`,
          },
        ],
      });
    }
    if (requestPath === `/api/repos/${REPOSITORY}`) {
      return sendJSON(response, {
        stargazers_count: 12,
        pushed_at: new Date().toISOString(),
        owner: { login: "acme" },
      });
    }
    if (requestPath.startsWith(`/raw/${REPOSITORY}/`)) {
      const packagePath = this.packages.get(this.currentRelease)!;
      const manifest = execFileSync("tar", ["-xOf", packagePath, "./manifest.yaml"], {
        encoding: "utf8",
      });
      response.writeHead(200, { "Content-Type": "text/yaml" });
      response.end(manifest);
      return;
    }
    if (requestPath === "/assets/checksums.txt") {
      const packagePath = this.packages.get(this.currentRelease)!;
      response.writeHead(200, { "Content-Type": "text/plain" });
      response.end(`${sha256(packagePath)}  ${path.basename(packagePath)}\n`);
      return;
    }
    if (requestPath.startsWith("/assets/")) {
      const packagePath = [...this.packages.values()].find(
        (candidate) => path.basename(candidate) === path.basename(requestPath),
      );
      if (packagePath) return sendFile(response, packagePath, "application/gzip");
    }
    response.writeHead(404);
    response.end("not found");
  }
}

function walkFiles(root: string, relative = ""): string[] {
  const directory = path.join(root, relative);
  return fs.readdirSync(directory, { withFileTypes: true }).flatMap((entry) => {
    const child = path.posix.join(relative.replaceAll(path.sep, "/"), entry.name);
    return entry.isDirectory() ? walkFiles(root, child) : [child];
  });
}

function sha256(filePath: string): string {
  return createHash("sha256").update(fs.readFileSync(filePath)).digest("hex");
}

function sendJSON(response: http.ServerResponse, body: unknown): void {
  response.writeHead(200, { "Content-Type": "application/json" });
  response.end(JSON.stringify(body));
}

function sendFile(response: http.ServerResponse, filePath: string, contentType: string): void {
  response.writeHead(200, { "Content-Type": contentType });
  fs.createReadStream(filePath).pipe(response);
}
