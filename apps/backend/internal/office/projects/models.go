// Package projects defines types and business logic for office projects.
package projects

import "github.com/kandev/kandev/internal/office/models"

// Project is a type alias for models.Project.
type Project = models.Project

// ProjectStatus is a type alias for models.ProjectStatus.
type ProjectStatus = models.ProjectStatus

// ProjectStatusActive, ProjectStatusCompleted, ProjectStatusOnHold, and
// ProjectStatusArchived are aliases for the models package constants.
const (
	ProjectStatusActive    = models.ProjectStatusActive
	ProjectStatusCompleted = models.ProjectStatusCompleted
	ProjectStatusOnHold    = models.ProjectStatusOnHold
	ProjectStatusArchived  = models.ProjectStatusArchived
)

// TaskCounts is a type alias for models.TaskCounts.
type TaskCounts = models.TaskCounts

// ProjectWithCounts is a type alias for models.ProjectWithCounts.
type ProjectWithCounts = models.ProjectWithCounts

// ValidProjectStatuses contains valid project status values.
var ValidProjectStatuses = models.ValidProjectStatuses
