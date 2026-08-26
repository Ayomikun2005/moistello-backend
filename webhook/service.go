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
)

type WebhookRegistration struct {
	ID        string    `json:"id"`
	UserID    string    `json:"user_id"`
	TargetURL string    `json:"target_url"`
	Secret    string    `json:"secret"`
	CreatedAt time.Time `json:"created_at"`
}

type WebhookRepository interface {
	Register(ctx context.Context, wh *WebhookRegistration) error
	GetByUserID(ctx context.Context, userID string) ([]WebhookRegistration, error)
	GetActiveWebhooks(ctx context.Context) ([]WebhookRegistration, error)
	GetByID(ctx context.Context, id string) (*WebhookRegistration, error)
}

type PostgresRepository struct {
	db *sql.DB
}

func NewPostgresRepository(db *sql.DB) *PostgresRepository {
	return &PostgresRepository{db: db}
}

func (r *PostgresRepository) Register(ctx context.Context, wh *WebhookRegistration) error {
	query := `
		INSERT INTO webhooks (id, user_id, target_url, secret, created_at)
		VALUES ($1, $2, $3, $4, $5)
	`
	_, err := r.db.ExecContext(ctx, query, wh.ID, wh.UserID, wh.TargetURL, wh.Secret, time.Now())
	return err
}

func (r *PostgresRepository) GetActiveWebhooks(ctx context.Context) ([]WebhookRegistration, error) {
	query := `SELECT id, user_id, target_url, secret, created_at FROM webhooks`
	rows, err := r.db.QueryContext(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var list []WebhookRegistration
	for rows.Next() {
		var wh WebhookRegistration
		if err := rows.Scan(&wh.ID, &wh.UserID, &wh.TargetURL, &wh.Secret, &wh.CreatedAt); err != nil {
			return nil, err
		}
		list = append(list, wh)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return list, nil
}

func (r *PostgresRepository) GetByID(ctx context.Context, id string) (*WebhookRegistration, error) {
	query := `SELECT id, user_id, target_url, secret, created_at FROM webhooks WHERE id = $1`
	var wh WebhookRegistration
	if err := r.db.QueryRowContext(ctx, query, id).Scan(&wh.ID, &wh.UserID, &wh.TargetURL, &wh.Secret, &wh.CreatedAt); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	return &wh, nil
}

func (r *PostgresRepository) GetByUserID(ctx context.Context, userID string) ([]WebhookRegistration, error) {
	query := `SELECT id, user_id, target_url, secret, created_at FROM webhooks WHERE user_id = $1`
	rows, err := r.db.QueryContext(ctx, query, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var list []WebhookRegistration
	for rows.Next() {
		var wh WebhookRegistration
		if err := rows.Scan(&wh.ID, &wh.UserID, &wh.TargetURL, &wh.Secret, &wh.CreatedAt); err != nil {
			return nil, err
		}
		list = append(list, wh)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return list, nil
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

// DispatchPayload delivers webhook payloads to active registrations with exponential backoff retries.
func (d *Dispatcher) DispatchPayload(ctx context.Context, payload interface{}, maxRetries int) error {
	webhooks, err := d.repo.GetActiveWebhooks(ctx)
	if err != nil {
		return fmt.Errorf("failed to load webhooks for dispatch: %w", err)
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("failed to marshal webhook payload: %w", err)
	}

	for _, wh := range webhooks {
		go d.deliverWithRetry(ctx, wh, body, maxRetries)
	}

	return nil
}

func (d *Dispatcher) deliverWithRetry(ctx context.Context, wh WebhookRegistration, body []byte, maxRetries int) {
	backoff := 100 * time.Millisecond

	for attempt := 1; attempt <= maxRetries; attempt++ {
		req, err := http.NewRequestWithContext(ctx, "POST", wh.TargetURL, bytes.NewBuffer(body))
		if err != nil {
			return
		}
		req.Header.Set("Content-Type", "application/json")
		if reqID, ok := ctx.Value("requestID").(string); ok && reqID != "" {
			req.Header.Set("X-Request-ID", reqID)
		}

		resp, err := d.httpClient.Do(req)
		if err == nil && resp.StatusCode >= 200 && resp.StatusCode < 300 {
			resp.Body.Close()
			return
		}

		if resp != nil {
			resp.Body.Close()
		}

		if attempt < maxRetries {
			time.Sleep(backoff)
			backoff *= 2
		}
	}
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
