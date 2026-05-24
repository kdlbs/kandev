import { AutomationEditor } from "@/components/automations/automation-editor";

type Props = {
  params: Promise<{ id: string; automationId: string }>;
};

export default async function AutomationEditorPage({ params }: Props) {
  const { id, automationId } = await params;
  return <AutomationEditor workspaceId={id} automationId={automationId} />;
}
