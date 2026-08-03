import { t } from "@/lib/i18n";
import { StateProvider } from "@/components/state-provider";
import { SystemPageShell } from "@/components/settings/system/system-page-shell";
import { LicensesList } from "@/components/settings/system/licenses-list";
import licenses from "@/generated/licenses.json";
import type { LicenseEntry } from "@/lib/types/system";

export default function SystemLicensesPage() {
  const entries = licenses as LicenseEntry[];

  return (
    <StateProvider initialState={{}}>
      <SystemPageShell
        title={t("system:navLicenses")}
        description={t("system:licensesPageDescription")}
      >
        <LicensesList entries={entries} />
      </SystemPageShell>
    </StateProvider>
  );
}
