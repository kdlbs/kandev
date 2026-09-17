package models

// OrchestratorRole owns the global identity and behavior of workspace coordinators.
type OrchestratorRole struct {
	Icon         string `json:"icon" db:"icon"`
	ID           string `json:"id" db:"id"`
	Name         string `json:"name" db:"name"`
	Instructions string `json:"instructions" db:"instructions"`
}

func ValidRoleIcon(icon string) bool {
	switch icon {
	case "", "💼", "🧭", "🤖", "🛠️", "🌱", "⭐":
		return true
	}
	return false
}
