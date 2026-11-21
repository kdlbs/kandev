package clarification

import taskmodels "github.com/kandev/kandev/internal/task/models"

// QuestionIdentities projects a newly asked bundle onto the fields compared
// when it replaces an older unanswered bundle of the same task.
func QuestionIdentities(questions []Question) []taskmodels.ClarificationQuestionIdentity {
	identities := make([]taskmodels.ClarificationQuestionIdentity, 0, len(questions))
	for _, question := range questions {
		identities = append(identities, taskmodels.ClarificationQuestionIdentity{
			ID:     question.ID,
			Prompt: question.Prompt,
		})
	}
	return identities
}
