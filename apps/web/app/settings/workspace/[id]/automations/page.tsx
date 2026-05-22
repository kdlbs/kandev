import { AutomationsListPage } from "@/components/automations/automations-list-page";

type Props = {
  params: Promise<{ id: string }>;
};

export default async function AutomationsPage({ params }: Props) {
  const { id } = await params;
  return <AutomationsListPage workspaceId={id} />;
}
