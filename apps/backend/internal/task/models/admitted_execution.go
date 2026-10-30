package models

// AdmittedExecution is a read projection of one existing active owner, not a
// second counter. Legacy unknowns remain explicit until their execution settles.
type AdmittedExecution struct {
	ID         string `db:"id"`
	Grade      string `db:"admitted_execution_grade"`
	GradeKnown bool   `db:"admitted_execution_grade_known"`
}
