import { execSync } from "node:child_process";

const isWindows = process.platform === "win32";

export type PlatformArch = "x64" | "arm64";
export type PlatformDir =
  | "linux-x64"
  | "macos-x64"
  | "macos-arm64"
  | "windows-x64"
  | "windows-arm64";

export function getBinaryName(base: string): string {
  return isWindows ? `${base}.exe` : base;
}

function getEffectiveArch(): PlatformArch {
  const platform = process.platform;
  const nodeArch = process.arch;
  if (platform === "darwin") {
    if (nodeArch === "arm64") return "arm64";
    try {
      const translated = execSync("sysctl -in sysctl.proc_translated", {
        encoding: "utf8",
      }).trim();
      if (translated === "1") return "arm64";
    } catch {}
    return "x64";
  }
  if (/arm/i.test(nodeArch)) return "arm64";
  if (platform === "win32") {
    const pa = process.env.PROCESSOR_ARCHITECTURE || "";
    const paw = process.env.PROCESSOR_ARCHITEW6432 || "";
    if (/arm/i.test(pa) || /arm/i.test(paw)) return "arm64";
  }
  return "x64";
}

export function getPlatformDir(): PlatformDir {
  const platform = process.platform;
  const arch = getEffectiveArch();
  if (platform === "linux" && arch === "x64") return "linux-x64";
  if (platform === "darwin" && arch === "x64") return "macos-x64";
  if (platform === "darwin" && arch === "arm64") return "macos-arm64";
  if (platform === "win32" && arch === "x64") return "windows-x64";
  if (platform === "win32" && arch === "arm64") return "windows-arm64";
  throw new Error(`Unsupported platform: ${platform}-${arch}`);
}
