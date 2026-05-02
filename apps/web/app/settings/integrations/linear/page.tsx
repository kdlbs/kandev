import { LinearIntegrationPage } from "@/components/linear/linear-settings";
import { StateProvider } from "@/components/state-provider";

export default function IntegrationsLinearPage() {
  return (
    <StateProvider initialState={{}}>
      <LinearIntegrationPage />
    </StateProvider>
  );
}
