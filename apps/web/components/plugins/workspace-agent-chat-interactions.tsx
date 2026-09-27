"use client";

import { useState } from "react";
import { useTranslation } from "react-i18next";
import { Button } from "@kandev/ui/button";
import { Card, CardContent, CardHeader, CardTitle } from "@kandev/ui/card";
import { Checkbox } from "@kandev/ui/checkbox";
import { Label } from "@kandev/ui/label";
import { Textarea } from "@kandev/ui/textarea";
import type {
  PluginManagedConversationClarificationAnswer,
  PluginManagedConversationInteraction,
} from "@kandev/plugin-sdk";

export type ClarificationAnswer = PluginManagedConversationClarificationAnswer;

// eslint-disable-next-line max-lines-per-function -- Permission and clarification controls share the pending interaction's answer state.
export function WorkspaceAgentChatInteractions({
  interaction,
  touchTargets,
  busy,
  onPermission,
  onClarification,
}: {
  interaction: PluginManagedConversationInteraction;
  touchTargets: boolean;
  busy: boolean;
  onPermission(optionId?: string): void;
  onClarification(answers: ClarificationAnswer[]): void;
}) {
  const { t } = useTranslation("plugins");
  const [selected, setSelected] = useState<Record<string, string[]>>({});
  const [text, setText] = useState<Record<string, string>>({});
  const questions = interaction.questions ?? [];
  const canAnswer =
    questions.length > 0 &&
    questions.every(
      (question) => (selected[question.id]?.length ?? 0) > 0 || Boolean(text[question.id]?.trim()),
    );

  return (
    <Card data-testid={`pending-interaction-${interaction.id}`} className="min-w-0">
      <CardHeader className="pb-2">
        <CardTitle className="text-sm">{interaction.title}</CardTitle>
        {interaction.context ? (
          <p className="break-words text-xs text-muted-foreground">{interaction.context}</p>
        ) : null}
      </CardHeader>
      <CardContent className="space-y-3">
        {interaction.kind === "permission" ? (
          <div className="flex flex-wrap gap-2">
            {(interaction.options ?? []).map((option) => (
              <Button
                key={option.id}
                type="button"
                size="sm"
                className={touchTargets ? "min-h-11" : "min-h-7"}
                disabled={busy}
                onClick={() => onPermission(option.id)}
              >
                {option.label}
              </Button>
            ))}
            <Button
              type="button"
              size="sm"
              variant="outline"
              className={touchTargets ? "min-h-11" : "min-h-7"}
              disabled={busy}
              onClick={() => onPermission()}
            >
              {t("managedChatCancelRequest")}
            </Button>
          </div>
        ) : (
          <div className="space-y-3">
            {questions.map((question) => (
              <fieldset key={question.id} className="space-y-2">
                <legend className="text-sm font-medium">{question.title}</legend>
                {question.prompt ? (
                  <p className="text-xs text-muted-foreground">{question.prompt}</p>
                ) : null}
                {(question.options ?? []).map((option) => {
                  const checked = selected[question.id]?.includes(option.id) ?? false;
                  return (
                    <Label
                      key={option.id}
                      className={`flex ${touchTargets ? "min-h-11" : "min-h-7"} items-center gap-3 rounded-md border px-3`}
                    >
                      <Checkbox
                        checked={checked}
                        onCheckedChange={(value) => {
                          setSelected((current) => {
                            const values = current[question.id] ?? [];
                            return {
                              ...current,
                              [question.id]: value
                                ? [...values, option.id]
                                : values.filter((item) => item !== option.id),
                            };
                          });
                        }}
                      />
                      <span>{option.label}</span>
                    </Label>
                  );
                })}
                <Textarea
                  aria-label={t("managedChatCustomAnswer", { question: question.title })}
                  className="min-h-20"
                  value={text[question.id] ?? ""}
                  onChange={(event) =>
                    setText((current) => ({ ...current, [question.id]: event.target.value }))
                  }
                />
              </fieldset>
            ))}
            <Button
              type="button"
              className={touchTargets ? "min-h-11" : "min-h-7"}
              disabled={busy || !canAnswer}
              onClick={() =>
                onClarification(
                  questions.map((question) => ({
                    questionId: question.id,
                    selectedOptions: selected[question.id] ?? [],
                    customText: text[question.id] ?? "",
                  })),
                )
              }
            >
              {t("managedChatSubmitAnswer")}
            </Button>
          </div>
        )}
      </CardContent>
    </Card>
  );
}
