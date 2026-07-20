import { SpritesSettings } from "@/components/settings/sprites-settings";
import { StateHydrator } from "@/components/state-hydrator";
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
    <>
      <StateHydrator initialState={initialState} />
      <SpritesSettings />
    </>
  );
}
