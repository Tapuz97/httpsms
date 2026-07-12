package handlers

import (
	"context"
	"fmt"
	"html"
	"net/mail"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/NdoleStudio/httpsms/pkg/emails"
	"github.com/NdoleStudio/httpsms/pkg/entities"
	"github.com/NdoleStudio/httpsms/pkg/requests"
	"github.com/NdoleStudio/httpsms/pkg/telemetry"
	"github.com/gofiber/fiber/v3"
	"github.com/google/uuid"
	"github.com/palantir/stacktrace"
	"gorm.io/gorm"
)

// EmailHandler handles plain email compose requests.
type EmailHandler struct {
	handler
	logger telemetry.Logger
	tracer telemetry.Tracer
	mailer emails.Mailer
	db     *gorm.DB
}

// NewEmailHandler creates a new EmailHandler.
func NewEmailHandler(logger telemetry.Logger, tracer telemetry.Tracer, mailer emails.Mailer, db *gorm.DB) (h *EmailHandler) {
	h = &EmailHandler{
		logger: logger.WithService(fmt.Sprintf("%T", h)),
		tracer: tracer,
		mailer: mailer,
		db:     db,
	}
	go h.resumePendingEmails()
	return h
}

// RegisterRoutes registers email routes.
func (h *EmailHandler) RegisterRoutes(router fiber.Router, middlewares ...fiber.Handler) {
	h.register(router, fiber.MethodPost, "/v1/emails/send", middlewares, h.PostSend)
	h.register(router, fiber.MethodGet, "/v1/emails", middlewares, h.Index)
	h.register(router, fiber.MethodDelete, "/v1/emails", middlewares, h.DeleteAll)
	h.register(router, fiber.MethodDelete, "/v1/emails/:emailID", middlewares, h.Delete)
}

// PostSend sends a plain email message through the configured SMTP service.
func (h *EmailHandler) PostSend(c fiber.Ctx) error {
	ctx, span := h.tracer.StartFromFiberCtx(c)
	defer span.End()

	ctxLogger := h.tracer.CtxLogger(h.logger, span)

	var request requests.EmailSend
	if err := c.Bind().Body(&request); err != nil {
		msg := fmt.Sprintf("cannot marshall [%s] into %T", c.Body(), request)
		ctxLogger.Warn(stacktrace.Propagate(err, msg))
		return h.responseBadRequest(c, err)
	}
	request = request.Sanitize()

	if errors := h.validate(request); len(errors) != 0 {
		ctxLogger.Warn(stacktrace.NewError(fmt.Sprintf("validation errors [%s], while sending email", errors.Encode())))
		return h.responseUnprocessableEntity(c, errors, "validation errors while sending email")
	}

	now := time.Now().UTC()
	message := &entities.EmailMessage{
		ID:        uuid.New(),
		UserID:    h.userIDFomContext(c),
		FromName:  request.FromName,
		To:        request.To,
		Subject:   request.Subject,
		Message:   request.Message,
		Status:    entities.EmailMessageStatusPending,
		CreatedAt: now,
		UpdatedAt: now,
	}
	if err := h.db.WithContext(ctx).Create(message).Error; err != nil {
		ctxLogger.Error(stacktrace.Propagate(err, "cannot store email attempt"))
		return h.responseInternalServerError(c)
	}

	go h.deliverEmail(message.ID)
	return h.responseOK(c, "email queued successfully", message)
}

func (h *EmailHandler) resumePendingEmails() {
	ctx := context.Background()
	h.db.WithContext(ctx).Model(&entities.EmailMessage{}).
		Where("status = ?", entities.EmailMessageStatusSending).
		Updates(map[string]any{"status": entities.EmailMessageStatusPending, "updated_at": time.Now().UTC()})

	var messages []*entities.EmailMessage
	if err := h.db.WithContext(ctx).Where("status = ?", entities.EmailMessageStatusPending).Find(&messages).Error; err != nil {
		h.logger.Error(stacktrace.Propagate(err, "cannot resume pending emails"))
		return
	}
	for _, message := range messages {
		go h.deliverEmail(message.ID)
	}
}

func (h *EmailHandler) deliverEmail(id uuid.UUID) {
	ctx := context.Background()
	now := time.Now().UTC()
	result := h.db.WithContext(ctx).Model(&entities.EmailMessage{}).
		Where("id = ? AND status = ?", id, entities.EmailMessageStatusPending).
		Updates(map[string]any{"status": entities.EmailMessageStatusSending, "updated_at": now})
	if result.Error != nil || result.RowsAffected != 1 {
		return
	}

	message := new(entities.EmailMessage)
	if err := h.db.WithContext(ctx).First(message, "id = ?", id).Error; err != nil {
		h.logger.Error(stacktrace.Propagate(err, "cannot load queued email"))
		return
	}
	body := strings.ReplaceAll(html.EscapeString(message.Message), "\n", "<br>")
	if err := h.mailer.Send(ctx, &emails.Email{FromName: message.FromName, ToEmail: message.To, Subject: message.Subject, Text: message.Message, HTML: "<p>" + body + "</p>"}); err != nil {
		failedAt := time.Now().UTC()
		failureReason := err.Error()
		h.db.WithContext(ctx).Model(message).Updates(map[string]any{
			"status": entities.EmailMessageStatusFailed, "failed_at": failedAt,
			"updated_at": failedAt, "failure_reason": failureReason,
		})
		h.logger.Error(stacktrace.Propagate(err, "cannot deliver queued email"))
		return
	}

	sentAt := time.Now().UTC()
	h.db.WithContext(ctx).Model(message).Updates(map[string]any{
		"status": entities.EmailMessageStatusSent, "sent_at": sentAt, "updated_at": sentAt,
	})
}

// Index returns the authenticated user's outbound email history.
func (h *EmailHandler) Index(c fiber.Ctx) error {
	ctx, span := h.tracer.StartFromFiberCtx(c)
	defer span.End()

	skip, _ := strconv.Atoi(c.Query("skip", "0"))
	limit, _ := strconv.Atoi(c.Query("limit", "20"))
	if skip < 0 {
		skip = 0
	}
	if limit < 1 {
		limit = 20
	}
	if limit > 100 {
		limit = 100
	}

	messages := make([]*entities.EmailMessage, 0, limit)
	err := h.db.WithContext(ctx).
		Where("user_id = ?", h.userIDFomContext(c)).
		Order("created_at DESC").
		Offset(skip).
		Limit(limit).
		Find(&messages).Error
	if err != nil {
		return h.responseInternalServerError(c)
	}

	return h.responseOK(c, fmt.Sprintf("fetched %d emails", len(messages)), messages)
}

// Delete removes one email history record owned by the authenticated user.
func (h *EmailHandler) Delete(c fiber.Ctx) error {
	ctx, span := h.tracer.StartFromFiberCtx(c)
	defer span.End()
	id, err := uuid.Parse(c.Params("emailID"))
	if err != nil {
		return h.responseBadRequest(c, err)
	}
	result := h.db.WithContext(ctx).Where("id = ? AND user_id = ?", id, h.userIDFomContext(c)).Delete(&entities.EmailMessage{})
	if result.Error != nil {
		return h.responseInternalServerError(c)
	}
	if result.RowsAffected == 0 {
		return h.responseNotFound(c, "email history record not found")
	}
	return h.responseNoContent(c, "email history record deleted")
}

// DeleteAll removes all email history owned by the authenticated user.
func (h *EmailHandler) DeleteAll(c fiber.Ctx) error {
	ctx, span := h.tracer.StartFromFiberCtx(c)
	defer span.End()
	if err := h.db.WithContext(ctx).Where("user_id = ?", h.userIDFomContext(c)).Delete(&entities.EmailMessage{}).Error; err != nil {
		return h.responseInternalServerError(c)
	}
	return h.responseNoContent(c, "email history cleared")
}

func (h *EmailHandler) validate(request requests.EmailSend) url.Values {
	errors := url.Values{}

	if _, err := mail.ParseAddress(request.To); err != nil {
		errors.Add("to", "must be a valid email address")
	}
	if len(request.FromName) > 100 || strings.ContainsAny(request.FromName, "\r\n") {
		errors.Add("from_name", "must be at most 100 characters and cannot contain line breaks")
	}
	if request.Subject == "" {
		errors.Add("subject", "is required")
	}
	if request.Message == "" {
		errors.Add("message", "is required")
	}

	return errors
}
