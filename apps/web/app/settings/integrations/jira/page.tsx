import { JiraIntegrationPage } from "@/components/jira/jira-settings";
import { StateProvider } from "@/components/state-provider";

export default function IntegrationsJiraPage() {
  return (
    <StateProvider initialState={{}}>
      <JiraIntegrationPage />
    </StateProvider>
  );
}
