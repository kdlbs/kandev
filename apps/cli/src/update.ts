import { spawn } from "node:child_process";
import https from "node:https";
import readline from "node:readline";

function requestJson<T>(url: string): Promise<T> {
  return new Promise((resolve, reject) => {
    const req = https.get(url, { headers: { "User-Agent": "kandev-npx" } }, (res) => {
      if (res.statusCode !== 200) {
        return reject(new Error(`HTTP ${res.statusCode} fetching ${url}`));
      }
      let body = "";
      res.on("data", (chunk) => (body += chunk));
      res.on("end", () => {
        try {
          resolve(JSON.parse(body) as T);
        } catch {
          reject(new Error(`Failed to parse JSON from ${url}`));
        }
      });
    });
    req.setTimeout(5000, () => {
      req.destroy(new Error(`Request timed out fetching ${url}`));
    });
    req.on("error", reject);
  });
}

async function getLatestNpmVersion(): Promise<string | undefined> {
  const data = await requestJson<{ "dist-tags"?: { latest?: string } }>(
    "https://registry.npmjs.org/kandev",
  );
  return data?.["dist-tags"]?.latest;
}

function compareVersions(a: string, b: string): number {
  const pa = String(a).replace(/^v/, "").split(".").map(Number);
  const pb = String(b).replace(/^v/, "").split(".").map(Number);
  for (let i = 0; i < Math.max(pa.length, pb.length); i += 1) {
    const av = pa[i] ?? 0;
    const bv = pb[i] ?? 0;
    if (av > bv) return 1;
    if (av < bv) return -1;
  }
  return 0;
}

function promptYesNo(question: string, defaultYes = false): Promise<boolean> {
  return new Promise<boolean>((resolve) => {
    if (!process.stdin.isTTY) {
      resolve(false);
      return;
    }
    const rl = readline.createInterface({
      input: process.stdin,
      output: process.stdout,
    });
    const suffix = defaultYes ? "[Y/n]" : "[y/N]";
    rl.question(`${question} ${suffix} `, (answer: string) => {
      rl.close();
      const normalized = String(answer || "")
        .trim()
        .toLowerCase();
      if (!normalized) {
        resolve(Boolean(defaultYes));
        return;
      }
      resolve(normalized === "y" || normalized === "yes");
    });
  });
}

export async function maybePromptForUpdate(currentVersion: string, args: string[]) {
  // Allow disabling update checks for CI or automation.
  if (process.env.KANDEV_SKIP_UPDATE === "1" || process.env.KANDEV_NO_UPDATE_PROMPT === "1") {
    return;
  }
  try {
    const latest = await getLatestNpmVersion();
    if (!latest) return;
    if (compareVersions(latest, currentVersion) <= 0) return;
    const wantsUpdate = await promptYesNo(
      `Update available: ${currentVersion} -> ${latest}. Update now?`,
      false,
    );
    if (!wantsUpdate) return;
    const env = { ...process.env, KANDEV_SKIP_UPDATE: "1" };
    const child = spawn("npx", ["kandev@latest", ...args], {
      stdio: "inherit",
      env,
    });
    child.on("exit", (code) => process.exit(code || 0));
    await new Promise(() => {});
  } catch {
    // ignore update errors
  }
}
