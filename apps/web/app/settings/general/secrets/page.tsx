import { SecretsSettings } from "@/components/settings/secrets-settings";
import { StateHydrator } from "@/components/state-hydrator";
import { listSecrets } from "@/lib/api/domains/secrets-api";

export default async function GeneralSecretsPage() {
  let initialState = {};
  try {
    const items = await listSecrets({ cache: "no-store" });
    initialState = {
      secrets: {
        items: items ?? [],
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
      <SecretsSettings />
    </>
  );
}
