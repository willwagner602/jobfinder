package web

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"net"
	"net/smtp"
	"strings"
)

type SMTPConfig struct {
	Addr     string
	Username string
	Password string
	From     string
}

type SMTPNotifier struct {
	Config SMTPConfig
}

func NewSMTPNotifier(cfg SMTPConfig) (*SMTPNotifier, error) {
	cfg.Addr = strings.TrimSpace(cfg.Addr)
	cfg.Username = strings.TrimSpace(cfg.Username)
	cfg.From = strings.TrimSpace(cfg.From)
	if cfg.From == "" {
		cfg.From = "jobs@willwagner.me"
	}
	var missing []string
	if cfg.Addr == "" {
		missing = append(missing, "JOBS_SMTP_ADDR")
	}
	if cfg.Username == "" {
		missing = append(missing, "JOBS_SMTP_USERNAME")
	}
	if cfg.Password == "" {
		missing = append(missing, "JOBS_SMTP_PASSWORD")
	}
	if len(missing) > 0 {
		return nil, fmt.Errorf("SMTP notifier missing required configuration: %s", strings.Join(missing, ", "))
	}
	return &SMTPNotifier{Config: cfg}, nil
}

func (n *SMTPNotifier) SendCode(ctx context.Context, email, code string) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}
	to := strings.TrimSpace(email)
	if to == "" {
		return errors.New("recipient email is required")
	}
	host, _, err := net.SplitHostPort(n.Config.Addr)
	if err != nil {
		host = n.Config.Addr
	}
	msg := strings.Join([]string{
		"From: " + n.Config.From,
		"To: " + to,
		"Subject: Your jobs.willwagner.me code",
		"MIME-Version: 1.0",
		"Content-Type: text/plain; charset=utf-8",
		"",
		"Your jobs.willwagner.me one-time code is: " + code,
		"",
		"This code expires in 15 minutes. If you did not request it, ignore this message.",
	}, "\r\n")
	auth := smtp.PlainAuth("", n.Config.Username, n.Config.Password, host)
	if strings.HasSuffix(n.Config.Addr, ":465") {
		return n.sendTLS(host, auth, to, []byte(msg))
	}
	return smtp.SendMail(n.Config.Addr, auth, n.Config.From, []string{to}, []byte(msg))
}

func (n *SMTPNotifier) sendTLS(host string, auth smtp.Auth, to string, msg []byte) error {
	conn, err := tls.Dial("tcp", n.Config.Addr, &tls.Config{ServerName: host, MinVersion: tls.VersionTLS12})
	if err != nil {
		return err
	}
	defer conn.Close()
	client, err := smtp.NewClient(conn, host)
	if err != nil {
		return err
	}
	defer client.Quit()
	if err := client.Auth(auth); err != nil {
		return err
	}
	if err := client.Mail(n.Config.From); err != nil {
		return err
	}
	if err := client.Rcpt(to); err != nil {
		return err
	}
	writer, err := client.Data()
	if err != nil {
		return err
	}
	if _, err := writer.Write(msg); err != nil {
		writer.Close()
		return err
	}
	return writer.Close()
}
