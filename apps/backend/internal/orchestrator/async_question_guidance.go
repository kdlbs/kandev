package orchestrator

func (s *Service) gateAsyncQuestionGuidance(options startCreatedSessionOptions) startCreatedSessionOptions {
	options.asyncQuestionsSupported = options.asyncQuestionsSupported && s.asyncQuestionsEnabled
	return options
}
