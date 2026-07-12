package entities

import (
	"time"

	"github.com/google/uuid"
)

// EmailMessageStatus is the delivery state of an outbound email.
type EmailMessageStatus string

const (
	EmailMessageStatusPending EmailMessageStatus = "pending"
	EmailMessageStatusSending EmailMessageStatus = "sending"
	EmailMessageStatusSent    EmailMessageStatus = "sent"
	EmailMessageStatusFailed  EmailMessageStatus = "failed"
)

// EmailMessage records an authenticated outbound email attempt.
type EmailMessage struct {
	ID            uuid.UUID          `json:"id" gorm:"primaryKey;type:uuid"`
	UserID        UserID             `json:"user_id" gorm:"index:idx_email_messages__user_id"`
	FromName      string             `json:"from_name"`
	To            string             `json:"to"`
	Subject       string             `json:"subject"`
	Message       string             `json:"message"`
	Status        EmailMessageStatus `json:"status" gorm:"index:idx_email_messages__status"`
	FailureReason *string            `json:"failure_reason,omitempty"`
	CreatedAt     time.Time          `json:"created_at"`
	UpdatedAt     time.Time          `json:"updated_at"`
	SentAt        *time.Time         `json:"sent_at,omitempty"`
	FailedAt      *time.Time         `json:"failed_at,omitempty"`
}

// TableName overrides the GORM table name.
func (EmailMessage) TableName() string {
	return "email_messages"
}
