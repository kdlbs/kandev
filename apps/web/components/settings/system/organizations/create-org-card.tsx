"use client";

import { useState } from "react";
import { useTranslation } from "react-i18next";
import { IconPlus } from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import { Input } from "@kandev/ui/input";
import { Label } from "@kandev/ui/label";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@kandev/ui/card";

type CreateOrgCardProps = {
  busy: boolean;
  onCreate: (name: string, onDone: () => void) => void;
};

export function CreateOrgCard({ busy, onCreate }: CreateOrgCardProps) {
  const { t } = useTranslation();
  const [name, setName] = useState("");

  return (
    <Card>
      <CardHeader>
        <CardTitle>{t("orgs:createTitle")}</CardTitle>
        <CardDescription>{t("orgs:createDescription")}</CardDescription>
      </CardHeader>
      <CardContent>
        <form
          className="flex flex-wrap items-end gap-2"
          onSubmit={(event) => {
            event.preventDefault();
            if (name.trim()) onCreate(name.trim(), () => setName(""));
          }}
        >
          <div className="min-w-56 flex-1 space-y-1">
            <Label htmlFor="new-org-name">{t("orgs:nameLabel")}</Label>
            <Input
              id="new-org-name"
              value={name}
              onChange={(event) => setName(event.target.value)}
              placeholder={t("orgs:namePlaceholder")}
            />
          </div>
          <Button type="submit" className="cursor-pointer" disabled={busy || !name.trim()}>
            <IconPlus className="size-4" aria-hidden />
            {t("orgs:create")}
          </Button>
        </form>
      </CardContent>
    </Card>
  );
}
