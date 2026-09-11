package domain

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"
)

type PollType string

const (
	SingleChoice   PollType = "single_choice"
	MultipleChoice PollType = "multiple_choice"
)

type PollStatus string

const (
	Draft  PollStatus = "draft"
	Active PollStatus = "active"
	Closed PollStatus = "closed"
)

type Option struct {
	ID       string `json:"id"`
	Text     string `json:"text"`
	Position int    `json:"-"`
}

type Poll struct {
	ID          string     `json:"id"`
	Slug        string     `json:"slug"`
	Question    string     `json:"question"`
	Type        PollType   `json:"type"`
	MaxChoices  int        `json:"max_choices"`
	Status      PollStatus `json:"status"`
	StartsAt    time.Time  `json:"starts_at"`
	EndsAt      time.Time  `json:"ends_at"`
	CreatedAt   time.Time  `json:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at"`
	PublishedAt *time.Time `json:"published_at,omitempty"`
	ClosedAt    *time.Time `json:"closed_at,omitempty"`
	Options     []Option   `json:"options"`
}

type CreatePoll struct {
	Question   string
	Type       PollType
	MaxChoices int
	StartsAt   time.Time
	EndsAt     time.Time
	Options    []string
}

func (in CreatePoll) Validate() error {
	question := strings.TrimSpace(in.Question)
	if n := utf8.RuneCountInString(question); n < 1 || n > 500 {
		return fmt.Errorf("%w: question must contain 1 to 500 characters", ErrInvalidInput)
	}
	if !in.StartsAt.Before(in.EndsAt) {
		return fmt.Errorf("%w: starts_at must be before ends_at", ErrInvalidInput)
	}
	if len(in.Options) < 2 || len(in.Options) > 20 {
		return fmt.Errorf("%w: poll must contain 2 to 20 options", ErrInvalidInput)
	}
	for _, option := range in.Options {
		if n := utf8.RuneCountInString(strings.TrimSpace(option)); n < 1 || n > 200 {
			return fmt.Errorf("%w: option must contain 1 to 200 characters", ErrInvalidInput)
		}
	}
	switch in.Type {
	case SingleChoice:
		if in.MaxChoices != 1 {
			return fmt.Errorf("%w: single_choice requires max_choices=1", ErrInvalidInput)
		}
	case MultipleChoice:
		if in.MaxChoices < 2 || in.MaxChoices > len(in.Options) {
			return fmt.Errorf("%w: multiple_choice max_choices must be between 2 and option count", ErrInvalidInput)
		}
	default:
		return fmt.Errorf("%w: unknown poll type", ErrInvalidInput)
	}
	return nil
}

func (p Poll) ValidateSelection(ids []string) error {
	if len(ids) == 0 || len(ids) > p.MaxChoices || (p.Type == SingleChoice && len(ids) != 1) {
		return ErrInvalidSelection
	}
	valid := make(map[string]struct{}, len(p.Options))
	for _, option := range p.Options {
		valid[option.ID] = struct{}{}
	}
	seen := make(map[string]struct{}, len(ids))
	for _, id := range ids {
		if !IsUUID(id) {
			return ErrInvalidSelection
		}
		if _, ok := seen[id]; ok {
			return ErrInvalidSelection
		}
		if _, ok := valid[id]; !ok {
			return ErrInvalidSelection
		}
		seen[id] = struct{}{}
	}
	return nil
}

func NewUUID() (string, error) {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	return fmt.Sprintf("%s-%s-%s-%s-%s", hex.EncodeToString(b[0:4]), hex.EncodeToString(b[4:6]), hex.EncodeToString(b[6:8]), hex.EncodeToString(b[8:10]), hex.EncodeToString(b[10:16])), nil
}

func IsUUID(s string) bool {
	if len(s) != 36 || s[8] != '-' || s[13] != '-' || s[18] != '-' || s[23] != '-' {
		return false
	}
	raw := strings.ReplaceAll(s, "-", "")
	_, err := hex.DecodeString(raw)
	return err == nil
}
