import { LinearSettings } from "@/components/linear/linear-settings";
import { StateProvider } from "@/components/state-provider";

export default async function WorkspaceLinearPage({ params }: { params: Promise<{ id: string }> }) {
  const { id } = await params;
  // `key={id}` forces a remount when the URL parameter changes so config /
  // form / testResult / teams from a previous workspace can never bleed into
  // the next one — Next.js doesn't unmount route segments on dynamic param
  // changes by default, only re-renders them.
  return (
    <StateProvider initialState={{}}>
      <LinearSettings key={id} workspaceId={id} />
    </StateProvider>
  );
}
