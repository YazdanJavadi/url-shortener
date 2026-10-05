package repository

import "errors"

var (
	// ErrNotFound is returned when no record matches the query.
	ErrNotFound = errors.New("record not found")
	// ErrDuplicateCode is returned by Save when the code already exists.
	ErrDuplicateCode = errors.New("duplicate short code")
)
