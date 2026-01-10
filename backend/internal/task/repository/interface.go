package repository

import (
	"context"

	"github.com/kandev/kandev/internal/task/models"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

// Repository defines the interface for task storage operations
type Repository interface {
	// Task operations
	CreateTask(ctx context.Context, task *models.Task) error
	GetTask(ctx context.Context, id string) (*models.Task, error)
	UpdateTask(ctx context.Context, task *models.Task) error
	DeleteTask(ctx context.Context, id string) error
	ListTasks(ctx context.Context, boardID string) ([]*models.Task, error)
	ListTasksByColumn(ctx context.Context, columnID string) ([]*models.Task, error)
	UpdateTaskState(ctx context.Context, id string, state v1.TaskState) error

	// Board operations
	CreateBoard(ctx context.Context, board *models.Board) error
	GetBoard(ctx context.Context, id string) (*models.Board, error)
	UpdateBoard(ctx context.Context, board *models.Board) error
	DeleteBoard(ctx context.Context, id string) error
	ListBoards(ctx context.Context) ([]*models.Board, error)

	// Column operations
	CreateColumn(ctx context.Context, column *models.Column) error
	GetColumn(ctx context.Context, id string) (*models.Column, error)
	UpdateColumn(ctx context.Context, column *models.Column) error
	DeleteColumn(ctx context.Context, id string) error
	ListColumns(ctx context.Context, boardID string) ([]*models.Column, error)

	// Close closes the repository (for database connections)
	Close() error
}

