#!/usr/bin/env node
import { createServer } from "node:http";
import { chmod, mkdir, mkdtemp, writeFile } from "node:fs/promises";
import { existsSync } from "node:fs";
import { tmpdir } from "node:os";
import { delimiter, dirname, join, resolve } from "node:path";
import { fileURLToPath } from "node:url";
import { spawn } from "node:child_process";

const __dirname = dirname(fileURLToPath(import.meta.url));
const desktopRoot = resolve(__dirname, "..");
const repoRoot = resolve(desktopRoot, "../..");

if (process.argv[2] === "--fake-runtime") {
  await runFakeRuntime(process.argv[3], process.argv.slice(4));
} else {
  await runSmoke();
}

async function runSmoke() {
  const appBinary = resolve(desktopRoot, "src-tauri/target/release/kandev-desktop");
  if (!existsSync(appBinary)) {
    throw new Error(`Missing desktop binary at ${appBinary}; run pnpm build first`);
  }

  const tmp = await mkdtemp(join(tmpdir(), "kandev-desktop-e2e-"));
  const runtimeDir = join(tmp, "runtime");
  const stateDir = join(tmp, "state");
  await mkdir(join(runtimeDir, "bin"), { recursive: true });
  await mkdir(stateDir, { recursive: true });

  await writeFakeRuntime(runtimeDir, stateDir);

  const launchedViaXvfb = !process.env.DISPLAY && commandExists("xvfb-run");
  const command = launchedViaXvfb ? "xvfb-run" : appBinary;
  const args = launchedViaXvfb ? ["-a", appBinary] : [];
  const child = spawn(command, args, {
    cwd: repoRoot,
    detached: process.platform !== "win32",
    env: {
      ...process.env,
      KANDEV_DESKTOP_RUNTIME_DIR: runtimeDir,
      WEBKIT_DISABLE_COMPOSITING_MODE: "1",
      NO_AT_BRIDGE: "1",
    },
    stdio: ["ignore", "pipe", "pipe"],
  });

  let stdout = "";
  let stderr = "";
  child.stdout?.on("data", (chunk) => {
    stdout += chunk;
  });
  child.stderr?.on("data", (chunk) => {
    stderr += chunk;
  });

  const failIfExited = () => {
    if (child.exitCode !== null) {
      throw new Error(`desktop app exited early with code ${child.exitCode}\n${stdout}\n${stderr}`);
    }
  };

  try {
    await waitForFile(join(stateDir, "health-requested"), 45_000, failIfExited);
    await waitForFile(join(stateDir, "root-requested"), 45_000, failIfExited);
  } finally {
    await stopProcess(child);
  }

  console.log("Desktop smoke passed: WebView requested / after backend health succeeded.");
}

async function writeFakeRuntime(runtimeDir, stateDir) {
  const fakeRuntime = join(runtimeDir, "bin", process.platform === "win32" ? "kandev.cmd" : "kandev");
  const agentctl = join(runtimeDir, "bin", process.platform === "win32" ? "agentctl.cmd" : "agentctl");
  const remoteHelpers = [
    ["agentctl-linux-amd64", "linux/amd64"],
    ["agentctl-linux-arm64", "linux/arm64"],
    ["agentctl-darwin-arm64", "darwin/arm64"],
    ["agentctl-darwin-amd64", "darwin/amd64"],
  ];

  if (process.platform === "win32") {
    await writeFile(
      fakeRuntime,
      `@echo off\r\nnode "${fileURLToPath(import.meta.url)}" --fake-runtime "${stateDir}" %*\r\n`,
    );
    await writeFile(agentctl, "@echo off\r\necho fake agentctl\r\n");
  } else {
    await writeFile(
      fakeRuntime,
      `#!/usr/bin/env bash\nexec node "${fileURLToPath(import.meta.url)}" --fake-runtime "${stateDir}" "$@"\n`,
    );
    await writeFile(agentctl, "#!/usr/bin/env bash\necho fake agentctl\n");
    await chmod(fakeRuntime, 0o755);
    await chmod(agentctl, 0o755);
  }

  for (const [name, platform] of remoteHelpers) {
    const helper = join(runtimeDir, "bin", name);
    await writeFile(helper, `#!/usr/bin/env bash\necho fake agentctl ${platform} helper\n`);
    if (process.platform !== "win32") {
      await chmod(helper, 0o755);
    }
  }
}

async function runFakeRuntime(stateDir, args) {
  const portIndex = args.indexOf("--port");
  const port = portIndex >= 0 ? Number(args[portIndex + 1]) : 0;
  const isHeadless = args.includes("--headless");

  if (!isHeadless || !Number.isInteger(port) || port <= 0) {
    await writeFile(join(stateDir, "invalid-args"), JSON.stringify(args));
    process.exit(2);
  }

  await writeFile(join(stateDir, "launched"), JSON.stringify({ args, port }));

  const server = createServer(async (req, res) => {
    if (req.url === "/health") {
      await writeFile(join(stateDir, "health-requested"), "1");
      const headers = { "content-type": "application/json" };
      if (process.env.KANDEV_DESKTOP_HEALTH_TOKEN) {
        headers["x-kandev-desktop-health-token"] = process.env.KANDEV_DESKTOP_HEALTH_TOKEN;
      }
      res.writeHead(200, headers);
      res.end('{"status":"ok"}');
      return;
    }

    if (req.url === "/") {
      await writeFile(join(stateDir, "root-requested"), "1");
      res.writeHead(200, { "content-type": "text/html" });
      res.end("<!doctype html><title>Kandev</title><main>Kandev desktop smoke</main>");
      return;
    }

    res.writeHead(404);
    res.end("not found");
  });

  await new Promise((resolveListen) => server.listen(port, "127.0.0.1", resolveListen));

  const stop = async () => {
    await writeFile(join(stateDir, "terminated"), "1");
    server.close(() => process.exit(0));
  };
  process.on("SIGTERM", stop);
  process.on("SIGINT", stop);
}

async function waitForFile(path, timeoutMs, tick) {
  const deadline = Date.now() + timeoutMs;
  while (Date.now() < deadline) {
    tick?.();
    if (existsSync(path)) {
      return;
    }
    await new Promise((resolveWait) => setTimeout(resolveWait, 250));
  }
  throw new Error(`Timed out waiting for ${path}`);
}

async function stopProcess(child) {
  if (child.exitCode !== null) {
    return;
  }
  if (process.platform === "win32") {
    child.kill();
  } else {
    process.kill(-child.pid, "SIGTERM");
  }
  await Promise.race([
    new Promise((resolveExit) => child.once("exit", resolveExit)),
    new Promise((resolveTimeout) => setTimeout(resolveTimeout, 5_000)),
  ]);
  if (child.exitCode === null) {
    if (process.platform === "win32") {
      child.kill("SIGKILL");
    } else {
      process.kill(-child.pid, "SIGKILL");
    }
  }
}

function commandExists(command) {
  const path = process.env.PATH ?? "";
  return path
    .split(delimiter)
    .filter(Boolean)
    .some((entry) => existsSync(join(entry, command)));
}
