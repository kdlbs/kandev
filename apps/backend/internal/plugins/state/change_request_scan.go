package state

import (
	"errors"
	"time"
)

// scanChangeRequest reads one row independently of how its timestamp columns
// are declared. SQLite DATETIME columns come back as time.Time when the stored
// text parses and as text otherwise, tables carried over from older shapes
// hold RFC3339 text (or empty text for an unset terminal instant), and
// Postgres returns time.Time. Every form is normalized to the record's
// RFC3339 layout; NULL or empty terminal instants become "".
func scanChangeRequest(scanner interface{ Scan(...any) error }) (ChangeRequestRecord, error) {
	var record ChangeRequestRecord
	var created, merged, closed, updated any
	err := scanner.Scan(&record.ID, &record.InstallationID, &record.WorkspaceID, &record.TaskID,
		&record.ProviderID, &record.ProviderHost, &record.RepositoryID, &record.Number,
		&record.URL, &record.Title, &record.State, &record.HeadBranch, &record.BaseBranch,
		&created, &merged, &closed, &updated)
	if err != nil {
		return record, err
	}
	if record.CreatedAt, err = requiredChangeRequestTimestamp(created); err != nil {
		return record, err
	}
	if record.UpdatedAt, err = requiredChangeRequestTimestamp(updated); err != nil {
		return record, err
	}
	if record.MergedAt, err = optionalChangeRequestTimestamp(merged); err != nil {
		return record, err
	}
	record.ClosedAt, err = optionalChangeRequestTimestamp(closed)
	return record, err
}

func requiredChangeRequestTimestamp(value any) (string, error) {
	parsed, err := optionalChangeRequestTimestamp(value)
	if err != nil {
		return "", err
	}
	if parsed == "" {
		return "", errors.New("plugin task change request: stored timestamp is missing")
	}
	return parsed, nil
}

var storedChangeRequestTimestampLayouts = []string{
	time.RFC3339Nano,
	"2006-01-02 15:04:05.999999999-07:00",
	"2006-01-02 15:04:05.999999999Z07:00",
	"2006-01-02 15:04:05",
}

func optionalChangeRequestTimestamp(value any) (string, error) {
	switch value := value.(type) {
	case nil:
		return "", nil
	case time.Time:
		if value.IsZero() {
			return "", errors.New("plugin task change request: stored timestamp is invalid")
		}
		return value.UTC().Format(changeRequestTimestampLayout), nil
	case []byte:
		return optionalChangeRequestTimestamp(string(value))
	case string:
		if value == "" {
			return "", nil
		}
		for _, layout := range storedChangeRequestTimestampLayouts {
			if parsed, err := time.Parse(layout, value); err == nil {
				return optionalChangeRequestTimestamp(parsed)
			}
		}
	}
	return "", errors.New("plugin task change request: stored timestamp is invalid")
}
