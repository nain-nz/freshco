package mailer

import (
	"fmt"
	"strings"

	"smartfood/internal/config"
)

type Message struct {
	To      string
	Subject string
	Text    string
	HTML    string
}

type Mailer interface {
	Name() string
	Send(msg Message) error
}

// New memilih pengirim email dari MAIL_PROVIDER: "console" atau "brevo".
func New(cfg config.Config) (Mailer, error) {
	switch strings.ToLower(cfg.MailProvider) {
	case "", "console":
		return Console{}, nil
	case "brevo":
		if cfg.BrevoAPIKey == "" || cfg.MailFromEmail == "" {
			return nil, fmt.Errorf("MAIL_PROVIDER=brevo butuh BREVO_API_KEY dan MAIL_FROM_EMAIL")
		}
		return NewBrevo(cfg.BrevoAPIKey, cfg.MailFromEmail, cfg.MailFromName), nil
	default:
		return nil, fmt.Errorf("MAIL_PROVIDER tidak dikenal: %q (pakai console atau brevo)", cfg.MailProvider)
	}
}
