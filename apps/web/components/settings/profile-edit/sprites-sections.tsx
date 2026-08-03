"use client";

import { useCallback } from "react";
import { IconPlus, IconTrash } from "@tabler/icons-react";
import { Badge } from "@kandev/ui/badge";
import { Button } from "@kandev/ui/button";
import { CardContent, CardHeader, CardTitle, CardDescription } from "@kandev/ui/card";
import { Input } from "@kandev/ui/input";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@kandev/ui/select";
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@kandev/ui/table";
import type { NetworkPolicyRule } from "@/lib/api/domains/settings-api";
import { SettingsCard } from "@/components/settings/settings-card";
import { useTranslation } from "react-i18next";

function PolicyRuleRow({
  rule,
  baselineRule,
  index,
  onUpdate,
  onRemove,
}: {
  rule: NetworkPolicyRule;
  baselineRule?: NetworkPolicyRule;
  index: number;
  onUpdate: (index: number, field: keyof NetworkPolicyRule, val: string) => void;
  onRemove: (index: number) => void;
}) {
  const { t } = useTranslation();
  return (
    <TableRow
      data-settings-dirty={!baselineRule || JSON.stringify(rule) !== JSON.stringify(baselineRule)}
      data-settings-dirty-level="container"
    >
      <TableCell>
        <Input
          value={rule.domain}
          onChange={(e) => onUpdate(index, "domain", e.target.value)}
          placeholder="*.example.com"
          className="text-sm"
          data-settings-dirty={!baselineRule || rule.domain !== baselineRule.domain}
        />
      </TableCell>
      <TableCell>
        <Select value={rule.action} onValueChange={(v) => onUpdate(index, "action", v)}>
          <SelectTrigger
            className="text-xs"
            data-settings-dirty={!baselineRule || rule.action !== baselineRule.action}
          >
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            <SelectItem value="allow">
              <Badge variant="default" className="bg-green-600">
                {t("executors:allow")}
              </Badge>
            </SelectItem>
            <SelectItem value="deny">
              <Badge variant="destructive">{t("executors:deny")}</Badge>
            </SelectItem>
          </SelectContent>
        </Select>
      </TableCell>
      <TableCell>
        <Input
          value={rule.include ?? ""}
          onChange={(e) => onUpdate(index, "include", e.target.value)}
          placeholder={t("executors:optionalPattern")}
          className="text-sm"
          data-settings-dirty={!baselineRule || rule.include !== baselineRule.include}
        />
      </TableCell>
      <TableCell>
        <Button
          variant="ghost"
          size="icon"
          onClick={() => onRemove(index)}
          className="cursor-pointer"
        >
          <IconTrash className="h-3.5 w-3.5 text-muted-foreground" />
        </Button>
      </TableCell>
    </TableRow>
  );
}

function PolicyRulesTable({
  rules,
  baselineRules,
  onUpdate,
  onRemove,
}: {
  rules: NetworkPolicyRule[];
  baselineRules?: NetworkPolicyRule[];
  onUpdate: (index: number, field: keyof NetworkPolicyRule, val: string) => void;
  onRemove: (index: number) => void;
}) {
  const { t } = useTranslation();
  return (
    <Table>
      <TableHeader>
        <TableRow>
          <TableHead>{t("executors:domain")}</TableHead>
          <TableHead className="w-[120px]">{t("executors:action")}</TableHead>
          <TableHead>{t("executors:include")}</TableHead>
          <TableHead className="w-[60px]" />
        </TableRow>
      </TableHeader>
      <TableBody>
        {rules.map((rule, idx) => (
          <PolicyRuleRow
            key={idx}
            rule={rule}
            baselineRule={baselineRules?.[idx]}
            index={idx}
            onUpdate={onUpdate}
            onRemove={onRemove}
          />
        ))}
      </TableBody>
    </Table>
  );
}

type NetworkPoliciesCardProps = {
  rules: NetworkPolicyRule[];
  baselineRules?: NetworkPolicyRule[];
  onRulesChange: (rules: NetworkPolicyRule[]) => void;
};

export function NetworkPoliciesCard({
  rules,
  baselineRules,
  onRulesChange,
}: NetworkPoliciesCardProps) {
  const { t } = useTranslation();
  const addRule = useCallback(() => {
    onRulesChange([...rules, { domain: "", action: "allow" }]);
  }, [rules, onRulesChange]);

  const removeRule = useCallback(
    (index: number) => {
      onRulesChange(rules.filter((_, i) => i !== index));
    },
    [rules, onRulesChange],
  );

  const updateRule = useCallback(
    (index: number, field: keyof NetworkPolicyRule, val: string) => {
      onRulesChange(rules.map((rule, i) => (i === index ? { ...rule, [field]: val } : rule)));
    },
    [rules, onRulesChange],
  );

  const isDirty =
    baselineRules !== undefined && JSON.stringify(rules) !== JSON.stringify(baselineRules);
  return (
    <SettingsCard isDirty={isDirty}>
      <CardHeader>
        <div className="flex items-center justify-between">
          <div>
            <CardTitle>{t("executors:networkPolicies")}</CardTitle>
            <CardDescription>{t("executors:defineNetworkAccessRulesAppliedWhen")}</CardDescription>
          </div>
          <Button
            type="button"
            variant="outline"
            size="sm"
            onClick={addRule}
            className="cursor-pointer"
          >
            <IconPlus className="mr-1 h-3.5 w-3.5" />
            {t("executors:addRule")}
          </Button>
        </div>
      </CardHeader>
      <CardContent>
        {rules.length === 0 ? (
          <p className="text-sm text-muted-foreground">
            {t("executors:noNetworkPolicyRulesConfigured")}
          </p>
        ) : (
          <PolicyRulesTable
            rules={rules}
            baselineRules={baselineRules}
            onUpdate={updateRule}
            onRemove={removeRule}
          />
        )}
      </CardContent>
    </SettingsCard>
  );
}
