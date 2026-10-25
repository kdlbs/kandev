import { useTranslation } from "react-i18next";
import { Button } from "@kandev/ui/button";

export type ListErrorStateProps = {
  retry: () => void;
};

/**
 * Shown on a coordinator route when the workspace's coordinator list itself
 * cannot be read (AC-COORDINATOR-NEEDS-YOU-006.4).
 */
export function ListErrorState({ retry }: ListErrorStateProps) {
  const { t } = useTranslation();
  return (
    <div className="space-y-2 py-8 text-center" data-testid="coordinator-list-error-state">
      <p className="text-sm">{t("coordinator:couldNotLoadCoordinators")}</p>
      <Button variant="outline" size="sm" onClick={retry}>
        {t("coordinator:tryAgain")}
      </Button>
    </div>
  );
}
