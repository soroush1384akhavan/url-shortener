package link

import "errors"

var ErrCodeCollision = errors.New("short code already exists")
var ErrCodeGenerationExhausted = errors.New("could not generate unique code")
// var ErrNotFound = errors.New("link not found")
var ErrInvalidURL = errors.New("invalid URL")