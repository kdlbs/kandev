// Package imagepreview defines the bounded raster-preview transport contract.
package imagepreview

import "net/http"

const MaxBytes = 10 * 1024 * 1024
const MaxPixels = 16_000_000

type Content struct {
	Bytes []byte
	MIME  string
}

type Error string

func (e Error) Error() string { return string(e) }

const (
	NotFound     Error = "not_found"
	Unavailable  Error = "unavailable"
	Unsupported  Error = "unsupported"
	InvalidImage Error = "invalid_image"
	TooLarge     Error = "too_large"
)

func (e Error) Status() int {
	switch e {
	case NotFound:
		return http.StatusNotFound
	case Unsupported:
		return http.StatusUnsupportedMediaType
	case InvalidImage:
		return http.StatusUnprocessableEntity
	case TooLarge:
		return http.StatusRequestEntityTooLarge
	default:
		return http.StatusServiceUnavailable
	}
}

func ParseError(code string) Error {
	switch value := Error(code); value {
	case NotFound, Unavailable, Unsupported, InvalidImage, TooLarge:
		return value
	default:
		return Unavailable
	}
}
