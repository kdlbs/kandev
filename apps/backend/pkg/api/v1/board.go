package v1

import "time"

// Board represents a Kanban board
type Board struct {
	ID          string    `json:"id"`
	Name        string    `json:"name"`
	Description *string   `json:"description,omitempty"`
	OwnerID     string    `json:"owner_id"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// CreateBoardRequest for creating a new board
type CreateBoardRequest struct {
	Name        string  `json:"name" binding:"required,max=255"`
	Description *string `json:"description,omitempty"`
}

// UpdateBoardRequest for updating a board
type UpdateBoardRequest struct {
	Name        *string `json:"name,omitempty" binding:"omitempty,max=255"`
	Description *string `json:"description,omitempty"`
}

