package backendapp

import (
	"context"
	"errors"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/kandev/kandev/internal/common/config"
	taskquestions "github.com/kandev/kandev/internal/task/questions"
)

type asyncQuestionStartupRecorder struct {
	order *[]string
}

func (r asyncQuestionStartupRecorder) Start(context.Context) {
	*r.order = append(*r.order, "async_questions")
}

func TestAsyncQuestionsDisabledCompositionLeavesNoServiceOrRoutes(t *testing.T) {
	services := &Services{AsyncQuestions: &taskquestions.Service{}}
	configureAsyncQuestions(routeParams{
		services: services,
		features: config.FeaturesConfig{AsyncQuestions: false},
	})
	require.Nil(t, services.AsyncQuestions)

	router := gin.New()
	taskquestions.RegisterRoutes(router, services.AsyncQuestions, nil, nil, nil)
	require.Empty(t, router.Routes())
}

func TestAsyncQuestionWorkerStartsAfterSuccessfulStartupPreparation(t *testing.T) {
	var order []string
	worker := asyncQuestionStartupRecorder{order: &order}
	err := prepareStartupAndStartAsyncQuestions(context.Background(), func(context.Context) error {
		order = append(order, "prepare")
		return nil
	}, worker)
	require.NoError(t, err)
	require.Equal(t, []string{"prepare", "async_questions"}, order)

	prepareErr := errors.New("startup preparation failed")
	order = nil
	err = prepareStartupAndStartAsyncQuestions(context.Background(), func(context.Context) error {
		order = append(order, "prepare")
		return prepareErr
	}, worker)
	require.ErrorIs(t, err, prepareErr)
	require.Equal(t, []string{"prepare"}, order)
}

func TestAsyncQuestionsDisabledStartupSkipsTypedNilWorker(t *testing.T) {
	services := &Services{}
	configureAsyncQuestions(routeParams{
		services: services,
		features: config.FeaturesConfig{AsyncQuestions: false},
	})
	require.Nil(t, services.AsyncQuestions)
	require.NoError(t, prepareStartupAndStartAsyncQuestions(
		context.Background(), func(context.Context) error { return nil }, configuredAsyncQuestionWorker(services.AsyncQuestions),
	))
}
