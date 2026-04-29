import { LinearSettings } from "@/components/linear/linear-settings";
import { StateProvider } from "@/components/state-provider";

export default async function WorkspaceLinearPage({ params }: { params: Promise<{ id: string }> }) {
  const { id } = await params;
  return (
    <StateProvider initialState={{}}>
      <LinearSettings workspaceId={id} />
    </StateProvider>
  );
}
