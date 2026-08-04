"use client";

import { useCallback, useEffect, useState } from "react";
import { useRouter } from "@/lib/routing/client-router";
import { Badge } from "@kandev/ui/badge";
import { Button } from "@kandev/ui/button";
import { Card, CardContent, CardHeader, CardTitle } from "@kandev/ui/card";
import { Input } from "@kandev/ui/input";
import { Label } from "@kandev/ui/label";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@kandev/ui/select";
import { IconCloud } from "@tabler/icons-react";
import { useTranslation } from "react-i18next";
import {
  createExecutor,
  createExecutorProfile,
  listCoderTemplates,
  type CoderTemplate,
} from "@/lib/api/domains/settings-api";

export function CoderCreatePage() {
  const { t } = useTranslation();
  const router = useRouter();
  const [name, setName] = useState("Coder");
  const [prefix, setPrefix] = useState("kandev");
  const [template, setTemplate] = useState("");
  const [templates, setTemplates] = useState<CoderTemplate[]>([]);
  const [error, setError] = useState("");
  const [saving, setSaving] = useState(false);

  useEffect(() => {
    void listCoderTemplates()
      .then((data) => {
        setTemplates(data.templates);
        if (data.templates.length > 0) setTemplate(data.templates[0].name);
      })
      .catch((cause: unknown) =>
        setError(cause instanceof Error ? cause.message : t("executors:coderTemplatesLoadFailed")),
      );
  }, [t]);

  const save = useCallback(async () => {
    if (!name.trim() || !template) return;
    setSaving(true);
    setError("");
    try {
      const executor = await createExecutor({
        name: name.trim(),
        type: "coder",
        config: { coder_template: template, coder_workspace_prefix: prefix.trim() || "kandev" },
      });
      const profile = await createExecutorProfile(executor.id, { name: name.trim() });
      router.push(`/settings/executors/${profile.id}`);
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : t("executors:coderCreateFailed"));
    } finally {
      setSaving(false);
    }
  }, [name, prefix, router, t, template]);

  return (
    <div className="space-y-8">
      <div className="flex items-center gap-2">
        <IconCloud className="h-5 w-5 text-muted-foreground" />
        <h2 className="text-2xl font-bold">{t("executors:newCoderExecutor")}</h2>
        <Badge variant="outline">{t("executors:coder")}</Badge>
      </div>
      <Card>
        <CardHeader>
          <CardTitle>{t("executors:workspace")}</CardTitle>
        </CardHeader>
        <CardContent className="space-y-5">
          <div className="space-y-2">
            <Label htmlFor="coder-name">{t("executors:name")}</Label>
            <Input id="coder-name" value={name} onChange={(event) => setName(event.target.value)} />
          </div>
          <div className="space-y-2">
            <Label>{t("executors:template")}</Label>
            <Select value={template} onValueChange={setTemplate} disabled={templates.length === 0}>
              <SelectTrigger data-testid="coder-template-select">
                <SelectValue placeholder={t("executors:selectCoderTemplate")} />
              </SelectTrigger>
              <SelectContent>
                {templates.map((item) => (
                  <SelectItem key={item.id} value={item.name}>
                    {item.display_name || item.name}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
          </div>
          <div className="space-y-2">
            <Label htmlFor="coder-prefix">{t("executors:coderWorkspacePrefix")}</Label>
            <Input
              id="coder-prefix"
              value={prefix}
              onChange={(event) => setPrefix(event.target.value)}
            />
            <p className="text-xs text-muted-foreground">
              {t("executors:coderWorkspacePrefixHelp")}
            </p>
          </div>
          {error ? <p className="text-sm text-destructive">{error}</p> : null}
          <div className="flex justify-end gap-2">
            <Button variant="outline" onClick={() => router.push("/settings/executors")}>
              {t("common:cancel")}
            </Button>
            <Button onClick={() => void save()} disabled={saving || !template || !name.trim()}>
              {saving ? t("executors:creating") : t("executors:createExecutor")}
            </Button>
          </div>
        </CardContent>
      </Card>
    </div>
  );
}
