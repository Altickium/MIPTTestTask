package domain

import "time"

type VoteStatus string

const (
	VoteRecorded        VoteStatus = "recorded"
	VoteAlreadyRecorded VoteStatus = "already_recorded"
)

type VoteCommand struct {
	PollID     string
	DeviceHash string
	OptionIDs  []string
	EndsAt     time.Time
}

type Totals struct {
	Participants int64
	Options      map[string]int64
	AsOf         time.Time
}

type Result struct {
	PollID       string
	Status       PollStatus
	Source       string
	AsOf         time.Time
	FinalizedAt  *time.Time
	Participants int64
	Options      map[string]int64
}
