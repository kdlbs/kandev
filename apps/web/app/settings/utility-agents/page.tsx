import { ConfigChatAgentSection } from "@/components/settings/config-chat-agent-section";
import { UtilityAgentsSection } from "@/components/settings/utility-agents-section";

export default function UtilityAgentsSettingsPage() {
  return (
    <div className="space-y-8">
      <UtilityAgentsSection />
      <ConfigChatAgentSection />
    </div>
  );
}
