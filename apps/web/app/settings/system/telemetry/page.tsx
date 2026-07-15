import { TelemetrySettings } from "@/components/settings/system/telemetry-settings";
import { SystemPageShell } from "@/components/settings/system/system-page-shell";

export default function TelemetryPage() {
  return (
    <SystemPageShell
      title="Telemetry"
      description="Strictly opt-in anonymous usage sharing. Nothing is sent unless you enable it."
    >
      <TelemetrySettings />
    </SystemPageShell>
  );
}
