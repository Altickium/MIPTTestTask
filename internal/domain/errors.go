package domain

import "errors"

var (
	ErrNotFound               = errors.New("not found")
	ErrInvalidInput           = errors.New("invalid input")
	ErrInvalidSelection       = errors.New("invalid selection")
	ErrInvalidTransition      = errors.New("invalid status transition")
	ErrNotActive              = errors.New("poll is not active")
	ErrAlreadyFinalized       = errors.New("poll is already finalized")
	ErrTemporarilyUnavailable = errors.New("temporarily unavailable")
)
