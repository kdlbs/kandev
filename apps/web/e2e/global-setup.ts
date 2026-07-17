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

  const spaIndex = path.join(WEB_DIR, "dist", "index.html");
  if (!fs.existsSync(spaIndex)) {
    throw new Error(`Vite web build not found: ${spaIndex}\nRun "make build-web" first.`);
  }

  // tests/plugins/plugins.spec.ts installs this package through the real
  // upload UI. Like the binaries above, this only checks existence — not
  // freshness — so rebuild after touching cmd/plugin-fixture (see
  // apps/backend/Makefile's e2e-plugin-package target).
  const pluginPackage = path.join(BACKEND_DIR, ".build", "kandev-plugin-e2e-1.0.0.tar.gz");
  if (!fs.existsSync(pluginPackage)) {
    throw new Error(
      `E2E fixture plugin package not found: ${pluginPackage}\nRun "make -C apps/backend e2e-plugin-package" first.`,
    );
  }
}
