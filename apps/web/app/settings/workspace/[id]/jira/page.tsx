import { JiraSettings } from "@/components/jira/jira-settings";
import { StateProvider } from "@/components/state-provider";

export default async function WorkspaceJiraPage({ params }: { params: Promise<{ id: string }> }) {
  const { id } = await params;
  return (
    <StateProvider initialState={{}}>
      <JiraSettings workspaceId={id} />
    </StateProvider>
  );
}
