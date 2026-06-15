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
}
