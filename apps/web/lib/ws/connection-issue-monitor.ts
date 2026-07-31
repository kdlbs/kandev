import type { ConnectionIssueSeverity, ConnectionStatus } from "@/lib/types/connection";

const UNSTABLE_DELAY_MS = 3_000;
const LOST_DELAY_MS = 10_000;

export class ConnectionIssueMonitor {
  private disposed = false;
  private offline = false;
  private severity: ConnectionIssueSeverity = "none";
  private unstableTimer: ReturnType<typeof setTimeout> | null = null;
  private lostTimer: ReturnType<typeof setTimeout> | null = null;

  constructor(private onSeverityChange: (severity: ConnectionIssueSeverity) => void) {}

  onStatusChange(status: ConnectionStatus) {
    if (this.disposed) {
      return;
    }

    if (status === "connected") {
      this.reset();
      return;
    }
    if (this.offline) return;

    this.offline = true;
    this.unstableTimer = setTimeout(() => this.reportSeverity("unstable"), UNSTABLE_DELAY_MS);
    this.lostTimer = setTimeout(() => this.reportSeverity("lost"), LOST_DELAY_MS);
  }

  dispose() {
    if (this.disposed) return;
    this.reset();
    this.disposed = true;
  }

  private reset() {
    this.offline = false;
    this.clearTimers();
    if (this.severity !== "none") this.reportSeverity("none");
  }

  private reportSeverity(severity: ConnectionIssueSeverity) {
    if (this.disposed) return;
    this.severity = severity;
    this.onSeverityChange(severity);
  }

  private clearTimers() {
    if (this.unstableTimer) clearTimeout(this.unstableTimer);
    if (this.lostTimer) clearTimeout(this.lostTimer);
    this.unstableTimer = null;
    this.lostTimer = null;
  }
}
