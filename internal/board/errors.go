package board

import (
	"errors"
	"fmt"
)

const (
	ReasonTaskNotFound      = "TASK_NOT_FOUND"
	ReasonHeld              = "CLAIM_HELD_BY_ANOTHER_SESSION"
	ReasonAlreadyHeld       = "CLAIM_ALREADY_HELD"
	ReasonClaimRequired     = "CLAIM_REQUIRED_FOR_DONE"
	ReasonNotClaimed        = "NOT_CLAIMED"
	ReasonInvalidTitle      = "INVALID_TITLE"
	ReasonInvalidGoal       = "INVALID_GOAL"
	ReasonInvalidAcceptance = "INVALID_ACCEPTANCE"
	ReasonInvalidStatus     = "INVALID_STATUS"
	ReasonInvalidTriage     = "INVALID_TRIAGE"
	ReasonInvalidPriority   = "INVALID_PRIORITY"
)

type Holder struct {
	Harness string `json:"harness"`
	Session string `json:"session"`
	Expires int64  `json:"expires"`
}

type Error struct {
	Reason  string  `json:"reason"`
	Message string  `json:"message"`
	Holder  *Holder `json:"holder,omitempty"`
}

func (e *Error) Error() string {
	if e.Holder != nil {
		return fmt.Sprintf("%s: %s (held by %s/%s)", e.Reason, e.Message, e.Holder.Harness, e.Holder.Session)
	}
	return e.Reason + ": " + e.Message
}

func AsError(err error) (*Error, bool) {
	var typed *Error
	if errors.As(err, &typed) {
		return typed, true
	}
	return nil, false
}
