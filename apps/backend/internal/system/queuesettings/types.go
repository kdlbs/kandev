package queuesettings

import (
	"errors"
	"fmt"

	"github.com/kandev/kandev/internal/orchestrator/messagequeue"
)

const (
	SettingsKey          = "message_queue"
	EnvironmentVariable  = "KANDEV_QUEUE_MAX_PER_SESSION"
	DefaultMaxPerSession = messagequeue.DefaultMaxPerSession
)

type Source string

const (
	SourceDefault     Source = "default"
	SourceSetting     Source = "setting"
	SourceEnvironment Source = "environment"
)

var (
	ErrValidation        = errors.New("message queue settings validation")
	ErrInvalidPersisted  = errors.New("invalid persisted message queue settings")
	ErrEnvironmentLocked = errors.New("message queue capacity is controlled by the environment")
	ErrTargetUnavailable = errors.New("message queue live target is unavailable")
)

type Settings struct {
	MaxPerSession int `json:"max_per_session"`
}

type Effective struct {
	MaxPerSession int    `json:"max_per_session"`
	Source        Source `json:"source"`
	Locked        bool   `json:"locked"`
}

type Response struct {
	Settings  Settings  `json:"settings"`
	Effective Effective `json:"effective"`
}

type Environment struct {
	Value   string
	Present bool
}

type Resolution struct {
	Response
	InvalidEnvironment bool
}

func DefaultSettings() Settings {
	return Settings{MaxPerSession: DefaultMaxPerSession}
}

func Validate(settings Settings) error {
	if settings.MaxPerSession < 0 {
		return fmt.Errorf("%w: max_per_session must be zero or greater", ErrValidation)
	}
	return nil
}
