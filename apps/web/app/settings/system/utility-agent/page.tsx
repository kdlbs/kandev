import { SystemPageShell } from "@/components/settings/system/system-page-shell";
import { UtilityAgentProfileSettings } from "@/components/settings/utility-agent-profile-settings";

export default function UtilityAgentPage() {
  return (
    <SystemPageShell
      title="Utility Agent"
      description="Choose the agent profile plugins use for lightweight, one-shot LLM calls."
    >
      <UtilityAgentProfileSettings />
    </SystemPageShell>
  );
}
