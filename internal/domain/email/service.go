package email

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
)

// Config holds the email sending configuration.
type Config struct {
	Provider    string // "brevo" or "sendgrid"
	APIKey      string
	FromAddress string
	FromName    string
}

// Service handles sending transactional emails via Brevo or SendGrid API.
type Service struct {
	config Config
	client *http.Client
}

func NewService(cfg Config) *Service {
	if cfg.FromAddress == "" {
		cfg.FromAddress = "noreply@moistello.com"
	}
	if cfg.FromName == "" {
		cfg.FromName = "Moistello"
	}
	return &Service{
		config: cfg,
		client: &http.Client{},
	}
}

// SendOTP sends a 6-digit verification code to the user's email.
func (s *Service) SendOTP(email, code string) error {
	subject := "Your Moistello verification code"
	body := fmt.Sprintf(`Your Moistello verification code is: <strong>%s</strong>

This code expires in 5 minutes. If you did not request this code, please ignore this email.`, code)
	return s.send(email, subject, body)
}

// SendBackupCodes sends the backup codes to the user's email after TOTP setup.
func (s *Service) SendBackupCodes(email string, codes []string) error {
	subject := "Your Moistello backup codes"
	body := `Save these backup codes in a secure place. Each code can be used only once to access your account if you lose your authenticator device.<br><br>`
	for _, c := range codes {
		body += fmt.Sprintf("<code>%s</code><br>", c)
	}
	body += `<br><strong>Keep these codes safe. They will not be shown again.</strong>`
	return s.send(email, subject, body)
}

// SendRecoveryCode sends a recovery code during the passwordless recovery flow.
func (s *Service) SendRecoveryCode(email, code string) error {
	subject := "Your Moistello recovery code"
	body := fmt.Sprintf(`Your Moistello recovery code is: <strong>%s</strong>

This code expires in 15 minutes. Use it to log in to your account. If you did not request this code, please secure your account immediately.`, code)
	return s.send(email, subject, body)
}

func (s *Service) send(to, subject, htmlBody string) error {
	switch s.config.Provider {
	case "brevo":
		return s.sendBrevo(to, subject, htmlBody)
	case "sendgrid":
		return s.sendSendGrid(to, subject, htmlBody)
	default:
		// Log the email in development — no actual sending
		fmt.Printf("[EMAIL] To: %s | Subject: %s | Body: %s\n", to, subject, htmlBody)
		return nil
	}
}

func (s *Service) sendBrevo(to, subject, htmlBody string) error {
	type brevoTo struct {
		Email string `json:"email"`
		Name  string `json:"name"`
	}
	type brevoPayload struct {
		Sender      map[string]string `json:"sender"`
		To          []brevoTo         `json:"to"`
		Subject     string            `json:"subject"`
		HTMLContent string            `json:"htmlContent"`
	}

	payload := brevoPayload{
		Sender: map[string]string{
			"name":  s.config.FromName,
			"email": s.config.FromAddress,
		},
		To:          []brevoTo{{Email: to}},
		Subject:     subject,
		HTMLContent: htmlBody,
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshaling brevo payload: %w", err)
	}

	req, err := http.NewRequest("POST", "https://api.brevo.com/v3/smtp/email", bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("creating brevo request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("api-key", s.config.APIKey)

	resp, err := s.client.Do(req)
	if err != nil {
		return fmt.Errorf("sending via brevo: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return fmt.Errorf("brevo API error: %s", resp.Status)
	}
	return nil
}

func (s *Service) sendSendGrid(to, subject, htmlBody string) error {
	type sendgridPersonalization struct {
		To []map[string]string `json:"to"`
	}
	type sendgridContent struct {
		Type  string `json:"type"`
		Value string `json:"value"`
	}
	type sendgridPayload struct {
		Personalizations []sendgridPersonalization `json:"personalizations"`
		From             map[string]string         `json:"from"`
		Subject          string                    `json:"subject"`
		Content          []sendgridContent         `json:"content"`
	}

	payload := sendgridPayload{
		Personalizations: []sendgridPersonalization{
			{To: []map[string]string{{"email": to}}},
		},
		From:    map[string]string{"email": s.config.FromAddress, "name": s.config.FromName},
		Subject: subject,
		Content: []sendgridContent{{Type: "text/html", Value: htmlBody}},
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshaling sendgrid payload: %w", err)
	}

	req, err := http.NewRequest("POST", "https://api.sendgrid.com/v3/mail/send", bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("creating sendgrid request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+s.config.APIKey)

	resp, err := s.client.Do(req)
	if err != nil {
		return fmt.Errorf("sending via sendgrid: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return fmt.Errorf("sendgrid API error: %s", resp.Status)
	}
	return nil
}

// ConfigFromEnv loads email config from environment variables.
func ConfigFromEnv() Config {
	provider := os.Getenv("MOISTELLO_EMAIL_PROVIDER")
	if provider == "" {
		provider = "brevo"
	}
	return Config{
		Provider:    provider,
		APIKey:      os.Getenv("MOISTELLO_EMAIL_API_KEY"),
		FromAddress: os.Getenv("MOISTELLO_EMAIL_FROM"),
		FromName:    "Moistello",
	}
}
