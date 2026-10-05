package urlservice

import "errors"

// ErrCodeGeneration is returned when no unique random code could be produced.
var ErrCodeGeneration = errors.New("failed to generate unique short code")
