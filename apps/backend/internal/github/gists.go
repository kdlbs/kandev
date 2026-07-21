package github

import "time"

// CreateGistInput is the payload for CreateGist.
type CreateGistInput struct {
	Description string
	Public      bool
	Files       map[string]GistFile
}

// GistFile is a single file entry inside a gist.
type GistFile struct {
	Content string
}

// GistResponse is the subset of the GitHub gist API response we care about.
type GistResponse struct {
	ID        string    `json:"id"`
	HTMLURL   string    `json:"html_url"`
	CreatedAt time.Time `json:"created_at"`
}
