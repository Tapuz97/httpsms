package requests

import "strings"

// EmailSend is the payload for sending a plain email message.
type EmailSend struct {
	FromName string `json:"from_name" example:"EmailSMS"`
	To       string `json:"to" example:"recipient@example.com"`
	Subject  string `json:"subject" example:"Subject"`
	Message  string `json:"message" example:"Message body"`
}

// Sanitize trims all user-provided email fields.
func (input *EmailSend) Sanitize() EmailSend {
	input.FromName = strings.TrimSpace(input.FromName)
	input.To = strings.TrimSpace(input.To)
	input.Subject = strings.TrimSpace(input.Subject)
	input.Message = strings.TrimSpace(input.Message)
	return *input
}
