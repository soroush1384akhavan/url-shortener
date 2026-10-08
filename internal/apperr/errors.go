package apperr

import "errors"

// shared error between link and store
var ErrCodeCollision = errors.New("short code already exists")