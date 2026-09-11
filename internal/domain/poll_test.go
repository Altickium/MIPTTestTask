package domain

import (
	"errors"
	"strings"
	"testing"
	"time"
)

func TestCreatePollValidate(t *testing.T) {
	now := time.Now()
	base := CreatePoll{Question: "Question?", Type: SingleChoice, MaxChoices: 1, StartsAt: now, EndsAt: now.Add(time.Minute), Options: []string{"A", "B"}}
	tests := []struct {
		name    string
		mutate  func(*CreatePoll)
		wantErr bool
	}{{"single", func(*CreatePoll) {}, false}, {"multiple", func(v *CreatePoll) { v.Type = MultipleChoice; v.MaxChoices = 2 }, false}, {"empty question", func(v *CreatePoll) { v.Question = " " }, true}, {"one option", func(v *CreatePoll) { v.Options = v.Options[:1] }, true}, {"bad time", func(v *CreatePoll) { v.EndsAt = v.StartsAt }, true}, {"single max", func(v *CreatePoll) { v.MaxChoices = 2 }, true}, {"multiple max", func(v *CreatePoll) { v.Type = MultipleChoice; v.MaxChoices = 3 }, true}, {"long option", func(v *CreatePoll) { v.Options[0] = strings.Repeat("x", 201) }, true}}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			v := base
			v.Options = append([]string(nil), base.Options...)
			tt.mutate(&v)
			err := v.Validate()
			if (err != nil) != tt.wantErr {
				t.Fatalf("Validate() err=%v wantErr=%v", err, tt.wantErr)
			}
		})
	}
}

func TestValidateSelection(t *testing.T) {
	p := Poll{Type: MultipleChoice, MaxChoices: 2, Options: []Option{{ID: "550e8400-e29b-41d4-a716-446655440000"}, {ID: "550e8400-e29b-41d4-a716-446655440001"}}}
	tests := []struct {
		name string
		ids  []string
		ok   bool
	}{{"one", []string{p.Options[0].ID}, true}, {"two", []string{p.Options[0].ID, p.Options[1].ID}, true}, {"duplicate", []string{p.Options[0].ID, p.Options[0].ID}, false}, {"foreign", []string{"550e8400-e29b-41d4-a716-446655440099"}, false}, {"empty", nil, false}}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := p.ValidateSelection(tt.ids)
			if tt.ok && err != nil {
				t.Fatal(err)
			}
			if !tt.ok && !errors.Is(err, ErrInvalidSelection) {
				t.Fatalf("got %v", err)
			}
		})
	}
}

func TestNewUUID(t *testing.T) {
	id, err := NewUUID()
	if err != nil {
		t.Fatal(err)
	}
	if !IsUUID(id) {
		t.Fatalf("invalid UUID %q", id)
	}
	if id[14] != '4' {
		t.Fatalf("not version 4: %s", id)
	}
}
