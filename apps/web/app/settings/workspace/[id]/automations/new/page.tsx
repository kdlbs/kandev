"use client";

import { AutomationEditor } from "@/components/automations/automation-editor";

type Props = {
  workspaceId: string;
};

export default function NewAutomationPage({ workspaceId }: Props) {
  return <AutomationEditor workspaceId={workspaceId} automationId={null} />;
}
