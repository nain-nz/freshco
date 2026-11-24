package mailer

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// Brevo mengirim email lewat API HTTPS (bukan SMTP, karena sebagian VM cloud
// memblokir port SMTP keluar).
type Brevo struct {
	apiKey    string
	fromEmail string
	fromName  string
	client    *http.Client
}

func NewBrevo(apiKey, fromEmail, fromName string) *Brevo {
	return &Brevo{
		apiKey:    apiKey,
		fromEmail: fromEmail,
		fromName:  fromName,
		client:    &http.Client{Timeout: 15 * time.Second},
	}
}

func (b *Brevo) Name() string { return "brevo" }

func (b *Brevo) Send(m Message) error {
	payload := map[string]any{
		"sender":      map[string]string{"name": b.fromName, "email": b.fromEmail},
		"to":          []map[string]string{{"email": m.To}},
		"subject":     m.Subject,
		"htmlContent": m.HTML,
		"textContent": m.Text,
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	req, err := http.NewRequest(http.MethodPost, "https://api.brevo.com/v3/smtp/email", bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("api-key", b.apiKey)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	resp, err := b.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		detail, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return fmt.Errorf("brevo menjawab %d: %s", resp.StatusCode, string(detail))
	}
	return nil
}
