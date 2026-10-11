import fs from "node:fs";
import os from "node:os";
import path from "node:path";
import { spawnSync } from "node:child_process";
import { afterEach, describe, expect, it } from "vitest";

const roots: string[] = [];
afterEach(() => {
  for (const root of roots.splice(0)) fs.rmSync(root, { recursive: true, force: true });
});

function run(script: string, args: string[], extra: Record<string, string> = {}) {
  const root = fs.mkdtempSync(path.join(os.tmpdir(), "worker-isolation-"));
  roots.push(root);
  const trace = path.join(root, "trace");
  fs.writeFileSync(
    path.join(root, "python3"),
    '#!/bin/sh\nprintf \'%s\\n\' "$@" > "$CHECK_TRACE"\n',
  );
  fs.writeFileSync(
    path.join(root, "pnpm"),
    "#!/bin/sh\nprintf 'unisolated-pnpm\\n' > \"$CHECK_TRACE\"\n",
  );
  fs.writeFileSync(
    path.join(root, "make"),
    '#!/bin/sh\nprintf "unisolated-make\\n" >> "$CHECK_TRACE"\n',
  );
  fs.writeFileSync(path.join(root, "docker"), "#!/bin/sh\nexit 1\n");
  for (const name of ["python3", "pnpm", "make", "docker"])
    fs.chmodSync(path.join(root, name), 0o755);
  const result = spawnSync("bash", [path.join(__dirname, script), ...args], {
    encoding: "utf8",
    cwd: path.resolve(__dirname, "../.."),
    timeout: 10_000,
    env: {
      ...process.env,
      PATH: `${root}:${process.env.PATH}`,
      CHECK_TRACE: trace,
      FULL_WORKER_CHECK_MODE: "isolated",
      FULL_WORKER_CHECK_INSIDE: "",
      KANDEV_E2E_ALLOW_UNSAFE_PARALLELISM: "",
      ...extra,
    },
  });
  return { ...result, trace: fs.existsSync(trace) ? fs.readFileSync(trace, "utf8") : "" };
}

describe("isolated worker E2E dispatch", () => {
  it.each(["run-e2e.sh", "run-raw-e2e.sh"])("dispatches %s before pnpm or build", (script) => {
    const result = run(script, ["--help"]);
    expect(result.status, result.stderr).toBe(0);
    expect(result.trace).toContain("--kind\nbrowser\n--\nbash\n");
    expect(result.trace).not.toContain("unisolated-pnpm");
  });

  it.each([["--workers=2"], ["-j2"], ["--shards", "2"], ["--project", "containers"]])(
    "rejects unsafe isolation arguments %s before dispatch",
    (...args: string[]) => {
      const result = run("run-e2e.sh", args);
      expect(result.status).toBe(2);
      expect(result.trace).toBe("");
    },
  );

  it("preserves package argument normalization before dispatch", () => {
    const result = run("run-e2e.sh", ["--", "--no-build", "--", "--grep", "a b"], {
      npm_lifecycle_event: "e2e:run",
    });
    expect(result.status, result.stderr).toBe(0);
    expect(result.trace).toContain("--host\n--no-build\n--\n--grep\na b\n");
  });
});
