import { parseServiceArgs } from "./args";
import { printServiceConfig } from "./config";
import { runLinuxService } from "./linux";
import { runMacosService } from "./macos";

export function printServiceHelp(): void {
  console.log(`kandev service — install kandev as an OS-managed service

Usage:
  kandev service install [--system] [--port <port>] [--home-dir <path>] [--no-boot-start]
  kandev service uninstall [--system]
  kandev service start|stop|restart|status [--system]
  kandev service logs [-f] [--system]
  kandev service config [--system]

Modes:
  default          User-level service.
                     Linux:  ~/.config/systemd/user/kandev.service
                     macOS:  ~/Library/LaunchAgents/com.kdlbs.kandev.plist
                   Runs as the current user. On Linux, only starts at boot
                   if 'loginctl enable-linger <user>' has been run.
  --system         System-level service. Requires sudo.
                     Linux:  /etc/systemd/system/kandev.service
                     macOS:  /Library/LaunchDaemons/com.kdlbs.kandev.plist
                   Starts at boot regardless of login state.

Flags:
  --port <port>        Backend port baked into the unit (KANDEV_SERVER_PORT).
  --home-dir <path>    KANDEV_HOME_DIR baked into the unit.
                       Defaults: ~/.kandev (user), /var/lib/kandev (system).
  --no-boot-start      (Linux user mode) Skip the enable-linger hint.
  -f, --follow         (logs) Stream logs instead of dumping the tail.

Updates:
  After 'npm update -g kandev' or 'brew upgrade kandev', re-run
  'kandev service install' to refresh paths in the unit file.
`);
}

export async function runServiceCommand(argv: string[]): Promise<void> {
  const args = parseServiceArgs(argv);
  if (args.showHelp) {
    printServiceHelp();
    return;
  }
  // 'config' is read-only and identical across platforms — handle here so we
  // don't need to duplicate it in both linux.ts and macos.ts.
  if (args.action === "config") {
    printServiceConfig(args);
    return;
  }
  switch (process.platform) {
    case "linux":
      return runLinuxService(args);
    case "darwin":
      return runMacosService(args);
    default:
      throw new Error(
        `kandev service is not yet supported on ${process.platform}. ` +
          `Currently supported: linux (systemd), darwin (launchd).`,
      );
  }
}
