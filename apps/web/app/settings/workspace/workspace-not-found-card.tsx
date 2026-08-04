"use client";

import { useTranslation } from "react-i18next";
import { Button } from "@kandev/ui/button";
import { Card, CardContent } from "@kandev/ui/card";

export function WorkspaceNotFoundCard({ onBack }: { onBack: () => void }) {
  const { t } = useTranslation();
  return (
    <div>
      <Card>
        <CardContent className="py-12 text-center">
          <p className="text-muted-foreground">{t("workspaces:workspaceNotFound")}</p>
          <Button className="mt-4" onClick={onBack}>
            {t("workspaces:backToWorkspaces")}
          </Button>
        </CardContent>
      </Card>
    </div>
  );
}
