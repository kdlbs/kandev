"use client";

import { useEffect } from "react";
import { useTranslation } from "react-i18next";
import { useRouter } from "@/lib/routing/client-router";
import { Card, CardContent } from "@kandev/ui/card";
import { Button } from "@kandev/ui/button";

export default function AgentEditPage() {
  const { t } = useTranslation();
  const router = useRouter();

  useEffect(() => {
    router.replace("/settings/agents");
  }, [router]);

  return (
    <Card>
      <CardContent className="py-12 text-center">
        <p className="text-sm text-muted-foreground">{t("agents:manageAgentsFromMainPage")}</p>
        <Button className="mt-4" onClick={() => router.push("/settings/agents")}>
          {t("agents:goToAgents")}
        </Button>
      </CardContent>
    </Card>
  );
}
