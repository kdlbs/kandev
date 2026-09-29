import { fetchJson } from "@/lib/api/client";
import type {
  PluginHumanInteractionResponseInput,
  PluginHostInteractionsApi,
} from "@kandev/plugin-sdk";

type ReceiptResponse = {
  id: string;
  interaction_id: string;
  resource_version: string;
  expires_at: string;
};

export async function issueHumanInteractionResponseReceipt(
  input: PluginHumanInteractionResponseInput,
): ReturnType<PluginHostInteractionsApi["issueResponseReceipt"]> {
  const response = await fetchJson<ReceiptResponse>(
    "/api/plugins/host/interactions/response-receipts",
    {
      init: {
        method: "POST",
        body: JSON.stringify({
          workspace_id: input.workspaceId,
          interaction_id: input.interactionId,
          expected_resource_version: input.expectedResourceVersion,
          kind: input.response.kind,
          ...(input.response.kind === "permission"
            ? { option_id: input.response.optionId, cancelled: input.response.cancelled }
            : {
                answers: input.response.answers.map((answer) => ({
                  question_id: answer.questionId,
                  selected_options: answer.selectedOptions,
                  custom_text: answer.customText,
                })),
              }),
        }),
      },
    },
  );
  return {
    id: response.id,
    interactionId: response.interaction_id,
    resourceVersion: response.resource_version,
    expiresAt: response.expires_at,
  };
}
