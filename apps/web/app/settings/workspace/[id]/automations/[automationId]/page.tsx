"use client";

import { AutomationEditor } from "@/components/automations/automation-editor";

type Props = {
  workspaceId: string;
  automationId: string;
};

export default function AutomationEditorPage({ workspaceId, automationId }: Props) {
  return <AutomationEditor workspaceId={workspaceId} automationId={automationId} />;
}
