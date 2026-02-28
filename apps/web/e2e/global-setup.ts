import fs from "node:fs";
import path from "node:path";

const BACKEND_DIR = path.resolve(__dirname, "../../../apps/backend");
const WEB_DIR = path.resolve(__dirname, "..");

export default function globalSetup() {
  const kandevBin = path.join(BACKEND_DIR, "bin", "kandev");
  const mockAgentBin = path.join(BACKEND_DIR, "bin", "mock-agent");

  for (const bin of [kandevBin, mockAgentBin]) {
    if (!fs.existsSync(bin)) {
      throw new Error(`Required binary not found: ${bin}\nRun "make build-backend" first.`);
    }
  }

  const standaloneServer = path.join(WEB_DIR, ".next/standalone/web/server.js");
  if (!fs.existsSync(standaloneServer)) {
    throw new Error(
      `Next.js standalone build not found: ${standaloneServer}\nRun "make build-web" first.`,
    );
  }

  // Standalone builds don't include static assets or public dir.
  // Symlink them so the standalone server can serve them.
  const staticSource = path.join(WEB_DIR, ".next/static");
  const staticTarget = path.join(WEB_DIR, ".next/standalone/web/.next/static");
  if (fs.existsSync(staticSource) && !fs.existsSync(staticTarget)) {
    fs.symlinkSync(staticSource, staticTarget, "dir");
  }

  const publicSource = path.join(WEB_DIR, "public");
  const publicTarget = path.join(WEB_DIR, ".next/standalone/web/public");
  if (fs.existsSync(publicSource) && !fs.existsSync(publicTarget)) {
    fs.symlinkSync(publicSource, publicTarget, "dir");
  }
}
