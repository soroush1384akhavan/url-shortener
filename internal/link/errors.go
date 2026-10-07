package link

import "errors"

var ErrCodeCollision = errors.New("short code already exists")
var ErrCodeGenerationExhausted = errors.New("could not generate unique code")