package webhook

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/lib/pq"
)

// WebhookRegistration is a webhook endpoint subscribed to a set of event
// types. An empty Events slice means the webhook receives every event type.
type WebhookRegistration struct {
	ID             string     `json:"id"`
	UserID         string     `json:"user_id"`
	TargetURL      string     `json:"target_url"`
	Secret         string     `json:"secret"`
	Events         []string   `json:"events"`
	IsActive       bool       `json:"is_active"`
	LastDeliveryAt *time.Time `json:"last_delivery_at,omitempty"`
	FailureCount   int        `json:"failure_count"`
	CreatedAt      time.Time  `json:"created_at"`
}

// DeliveryLog is a single webhook delivery attempt (or filter skip).
type DeliveryLog struct {
	ID         string    `json:"id"`
	WebhookID  string    `json:"webhook_id"`
	EventType  string    `json:"event_type"`
	Status     string    `json:"status"`
	StatusCode int       `json:"status_code,omitempty"`
	Attempt    int       `json:"attempt"`
	Error      string    `json:"error,omitempty"`
	CreatedAt  time.Time `json:"created_at"`
}

// Delivery statuses recorded in webhook_deliveries.
const (
	DeliveryStatusDelivered = "delivered"
	DeliveryStatusFailed    = "failed"
	DeliveryStatusSkipped   = "skipped"
)

type WebhookRepository interface {
	Register(ctx context.Context, wh *WebhookRegistration) error
	GetByUserID(ctx context.Context, userID string) ([]WebhookRegistration, error)
	GetActiveWebhooks(ctx context.Context) ([]WebhookRegistration, error)
	GetByID(ctx context.Context, id string) (*WebhookRegistration, error)
	Delete(ctx context.Context, id string) error
	// UpdateDeliveryOutcome records a delivery result: sets last_delivery_at
	// and increments failure_count on failure.
	UpdateDeliveryOutcome(ctx context.Context, id string, success bool) error
	// LogDelivery appends an entry to the webhook delivery log.
	LogDelivery(ctx context.Context, log *DeliveryLog) error
	// ListDeliveries returns a paginated delivery history for a webhook.
	ListDeliveries(ctx context.Context, webhookID string, page, limit int) ([]DeliveryLog, int, error)
}

type PostgresRepository struct {
	db *sql.DB
}

func NewPostgresRepository(db *sql.DB) *PostgresRepository {
	return &PostgresRepository{db: db}
}

func (r *PostgresRepository) Register(ctx context.Context, wh *WebhookRegistration) error {
	createdAt := wh.CreatedAt
	if createdAt.IsZero() {
		createdAt = time.Now()
	}
	query := `
		INSERT INTO webhooks (id, user_id, url, secret_hash, events, is_active, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
	`
	_, err := r.db.ExecContext(ctx, query, wh.ID, wh.UserID, wh.TargetURL, wh.Secret, pq.Array(wh.Events), wh.IsActive, createdAt)
	return err
}

func (r *PostgresRepository) GetActiveWebhooks(ctx context.Context) ([]WebhookRegistration, error) {
	query := `SELECT id, user_id, url, secret_hash, events, is_active, last_delivery_at, failure_count, created_at
		FROM webhooks WHERE is_active = TRUE`
	rows, err := r.db.QueryContext(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var list []WebhookRegistration
	for rows.Next() {
		wh, err := scanWebhook(rows)
		if err != nil {
			return nil, err
		}
		list = append(list, *wh)
	}
	return list, nil
}

func (r *PostgresRepository) GetByID(ctx context.Context, id string) (*WebhookRegistration, error) {
	query := `SELECT id, user_id, url, secret_hash, events, is_active, last_delivery_at, failure_count, created_at
		FROM webhooks WHERE id = $1`
	return scanWebhook(r.db.QueryRowContext(ctx, query, id))
}

func (r *PostgresRepository) GetByUserID(ctx context.Context, userID string) ([]WebhookRegistration, error) {
	query := `SELECT id, user_id, url, secret_hash, events, is_active, last_delivery_at, failure_count, created_at
		FROM webhooks WHERE user_id = $1 ORDER BY created_at DESC`
	rows, err := r.db.QueryContext(ctx, query, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var list []WebhookRegistration
	for rows.Next() {
		wh, err := scanWebhook(rows)
		if err != nil {
			return nil, err
		}
		list = append(list, *wh)
	}
	return list, nil
}

func (r *PostgresRepository) Delete(ctx context.Context, id string) error {
	result, err := r.db.ExecContext(ctx, `DELETE FROM webhooks WHERE id = $1`, id)
	if err != nil {
		return err
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rows == 0 {
		return sql.ErrNoRows
	}
	return nil
}

func (r *PostgresRepository) UpdateDeliveryOutcome(ctx context.Context, id string, success bool) error {
	query := `UPDATE webhooks SET last_delivery_at = NOW(), failure_count = CASE WHEN $2 THEN 0 ELSE failure_count + 1 END WHERE id = $1`
	_, err := r.db.ExecContext(ctx, query, id, success)
	return err
}

func (r *PostgresRepository) LogDelivery(ctx context.Context, log *DeliveryLog) error {
	query := `
		INSERT INTO webhook_deliveries (webhook_id, event_type, status, status_code, attempt, error, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
	`
	_, err := r.db.ExecContext(ctx, query, log.WebhookID, log.EventType, log.Status, nullInt(log.StatusCode), log.Attempt, nullString(log.Error), time.Now())
	return err
}

func (r *PostgresRepository) ListDeliveries(ctx context.Context, webhookID string, page, limit int) ([]DeliveryLog, int, error) {
	if page < 1 {
		page = 1
	}
	if limit < 1 || limit > 100 {
		limit = 20
	}
	offset := (page - 1) * limit

	var total int
	if err := r.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM webhook_deliveries WHERE webhook_id = $1`, webhookID).Scan(&total); err != nil {
		return nil, 0, err
	}

	query := `SELECT id, webhook_id, event_type, status, COALESCE(status_code, 0), attempt, COALESCE(error, ''), created_at
		FROM webhook_deliveries WHERE webhook_id = $1 ORDER BY created_at DESC LIMIT $2 OFFSET $3`
	rows, err := r.db.QueryContext(ctx, query, webhookID, limit, offset)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var list []DeliveryLog
	for rows.Next() {
		var d DeliveryLog
		if err := rows.Scan(&d.ID, &d.WebhookID, &d.EventType, &d.Status, &d.StatusCode, &d.Attempt, &d.Error, &d.CreatedAt); err != nil {
			return nil, 0, err
		}
		list = append(list, d)
	}
	return list, total, rows.Err()
}

type scanner interface {
	Scan(dest ...interface{}) error
}

// scanWebhook reads a webhook row. The secret_hash column stores the raw HMAC
// secret used to verify incoming deliveries (existing convention).
func scanWebhook(row scanner) (*WebhookRegistration, error) {
	var wh WebhookRegistration
	var events []string
	var lastDeliveryAt sql.NullTime
	err := row.Scan(&wh.ID, &wh.UserID, &wh.TargetURL, &wh.Secret, &events, &wh.IsActive, &lastDeliveryAt, &wh.FailureCount, &wh.CreatedAt)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	wh.Events = events
	if lastDeliveryAt.Valid {
		t := lastDeliveryAt.Time
		wh.LastDeliveryAt = &t
	}
	return &wh, nil
}

func nullInt(v int) interface{} {
	if v == 0 {
		return nil
	}
	return v
}

func nullString(v string) interface{} {
	if v == "" {
		return nil
	}
	return v
}

type Dispatcher struct {
	repo       WebhookRepository
	httpClient *http.Client
}

func NewDispatcher(repo WebhookRepository) *Dispatcher {
	return &Dispatcher{
		repo: repo,
		httpClient: &http.Client{
			Timeout: 5 * time.Second,
		},
	}
}

// DispatchPayload delivers webhook payloads to active registrations that
// subscribed to the given event type. Webhooks with no event subscription
// receive every event. Delivery failures are retried with exponential backoff
// and every attempt is recorded in the delivery log.
func (d *Dispatcher) DispatchPayload(ctx context.Context, eventType string, payload interface{}, maxRetries int) error {
	webhooks, err := d.repo.GetActiveWebhooks(ctx)
	if err != nil {
		return fmt.Errorf("failed to load webhooks for dispatch: %w", err)
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("failed to marshal webhook payload: %w", err)
	}

	for _, wh := range webhooks {
		if !wh.SubscribesTo(eventType) {
			d.recordSkip(ctx, wh, eventType)
			continue
		}
		go d.deliverWithRetry(ctx, wh, eventType, body, maxRetries)
	}

	return nil
}

// SubscribesTo reports whether the registration should receive the given
// event type. An empty subscription list means "all events".
func (w *WebhookRegistration) SubscribesTo(eventType string) bool {
	if len(w.Events) == 0 {
		return true
	}
	for _, e := range w.Events {
		if e == eventType {
			return true
		}
	}
	return false
}

func (d *Dispatcher) recordSkip(ctx context.Context, wh WebhookRegistration, eventType string) {
	_ = d.repo.LogDelivery(ctx, &DeliveryLog{
		WebhookID: wh.ID,
		EventType: eventType,
		Status:    DeliveryStatusSkipped,
		Attempt:   0,
	})
}

func (d *Dispatcher) deliverWithRetry(ctx context.Context, wh WebhookRegistration, eventType string, body []byte, maxRetries int) {
	backoff := 100 * time.Millisecond

	for attempt := 1; attempt <= maxRetries; attempt++ {
		success, statusCode, errMsg := d.deliverOnce(ctx, wh, body)
		if success {
			d.recordDelivery(ctx, wh, eventType, DeliveryStatusDelivered, statusCode, attempt, "")
			return
		}

		if attempt == maxRetries {
			d.recordDelivery(ctx, wh, eventType, DeliveryStatusFailed, statusCode, attempt, errMsg)
			return
		}

		time.Sleep(backoff)
		backoff *= 2
	}
}

func (d *Dispatcher) deliverOnce(ctx context.Context, wh WebhookRegistration, body []byte) (bool, int, string) {
	req, err := http.NewRequestWithContext(ctx, "POST", wh.TargetURL, bytes.NewBuffer(body))
	if err != nil {
		return false, 0, err.Error()
	}
	req.Header.Set("Content-Type", "application/json")
	if reqID, ok := ctx.Value("requestID").(string); ok && reqID != "" {
		req.Header.Set("X-Request-ID", reqID)
	}

	resp, err := d.httpClient.Do(req)
	if err != nil {
		return false, 0, err.Error()
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		return true, resp.StatusCode, ""
	}
	return false, resp.StatusCode, fmt.Sprintf("unexpected status code %d", resp.StatusCode)
}

func (d *Dispatcher) recordDelivery(ctx context.Context, wh WebhookRegistration, eventType, status string, statusCode, attempt int, errMsg string) {
	_ = d.repo.UpdateDeliveryOutcome(ctx, wh.ID, status == DeliveryStatusDelivered)
	_ = d.repo.LogDelivery(ctx, &DeliveryLog{
		WebhookID:  wh.ID,
		EventType:  eventType,
		Status:     status,
		StatusCode: statusCode,
		Attempt:    attempt,
		Error:      errMsg,
	})
}

// SignWebhookPayload computes an HMAC-SHA256 signature for the given payload
// using the webhook secret. The signature is returned as a hex string.
func SignWebhookPayload(payload []byte, secret string) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(payload)
	return hex.EncodeToString(mac.Sum(nil))
}

// VerifyWebhookSignature verifies the HMAC-SHA256 signature of a webhook payload
// using constant-time comparison to prevent timing attacks.
func VerifyWebhookSignature(payload []byte, signature, secret string) bool {
	if len(signature) == 0 {
		return false
	}
	expected := SignWebhookPayload(payload, secret)
	return constantTimeCompare([]byte(expected), []byte(signature))
}

// constantTimeCompare reports whether a and b are equal in constant time.
func constantTimeCompare(a, b []byte) bool {
	return subtle.ConstantTimeCompare(a, b) == 1
}

// VerifySignature reports whether two hex-encoded signatures are equal, in
// constant time. Non-hex or length-mismatched inputs never match.
func VerifySignature(expected, signature string) bool {
	expectedBytes, err := hex.DecodeString(expected)
	if err != nil {
		return false
	}
	sigBytes, err := hex.DecodeString(signature)
	if err != nil {
		return false
	}
	return constantTimeCompare(expectedBytes, sigBytes)
}
