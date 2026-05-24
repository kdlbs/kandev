import { AutomationEditor } from "@/components/automations/automation-editor";

type Props = {
  params: Promise<{ id: string }>;
};

export default async function NewAutomationPage({ params }: Props) {
  const { id } = await params;
  return <AutomationEditor workspaceId={id} automationId={null} />;
}
