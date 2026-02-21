import { SpritesSettings } from "@/components/settings/sprites-settings";
import { StateProvider } from "@/components/state-provider";
import { getSpritesStatus, listSpritesInstances } from "@/lib/api/domains/sprites-api";

export default async function GeneralSpritesPage() {
  let initialState = {};
  try {
    const [status, instances] = await Promise.all([
      getSpritesStatus(undefined, { cache: "no-store" }),
      listSpritesInstances(undefined, { cache: "no-store" }),
    ]);
    initialState = {
      sprites: {
        status,
        instances: instances ?? [],
        loaded: true,
        loading: false,
      },
    };
  } catch {
    initialState = {};
  }

  return (
    <StateProvider initialState={initialState}>
      <SpritesSettings />
    </StateProvider>
  );
}
