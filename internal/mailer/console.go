package mailer

import "log"

type Console struct{}

func (Console) Name() string { return "console" }

func (Console) Send(msg Message) error {
	log.Printf("[mail:console] ke=%s | %s\n%s", msg.To, msg.Subject, msg.Text)
	return nil
}
