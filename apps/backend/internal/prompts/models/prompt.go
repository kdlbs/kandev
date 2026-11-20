package models

import "time"

type Prompt struct {
	ID        string
	Name      string
	Content   string
	Builtin   bool
	CreatedAt time.Time
	UpdatedAt time.Time
}
