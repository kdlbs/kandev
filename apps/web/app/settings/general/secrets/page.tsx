import { SecretsSettings } from "@/components/settings/secrets-settings";
import { StateProvider } from "@/components/state-provider";
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
    <StateProvider initialState={initialState}>
      <SecretsSettings />
    </StateProvider>
  );
}
