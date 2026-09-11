// Package mail sends real email over SMTP (implicit TLS on 465, STARTTLS otherwise).
package mail

import (
	"crypto/tls"
	"encoding/base64"
	"fmt"
	"net"
	"net/smtp"
	"strings"
)

// Sender delivers email via a configured SMTP server.
type Sender struct {
	host string
	port string
	user string
	pass string
	from string
}

// NewSender creates an SMTP sender. Returns nil when SMTP is not configured,
// so callers can fall back to a dev behavior.
func NewSender(host, port, user, pass, from string) *Sender {
	if host == "" || user == "" || pass == "" {
		return nil
	}
	if port == "" {
		port = "465"
	}
	if from == "" {
		from = user
	}
	return &Sender{host: host, port: port, user: user, pass: pass, from: from}
}

// addr returns host:port.
func (s *Sender) addr() string { return net.JoinHostPort(s.host, s.port) }

// plainFrom extracts the bare address from a "Name <addr>" from-header.
func (s *Sender) plainFrom() string {
	from := s.from
	if i := strings.Index(from, "<"); i >= 0 {
		if j := strings.Index(from[i:], ">"); j > 0 {
			return strings.TrimSpace(from[i+1 : i+j])
		}
	}
	return from
}

// Send delivers a plain-text + HTML email to one recipient.
func (s *Sender) Send(to, subject, textBody, htmlBody string) error {
	var msg strings.Builder
	msg.WriteString("From: " + s.from + "\r\n")
	msg.WriteString("To: " + to + "\r\n")
	msg.WriteString("Subject: =?UTF-8?B?" + base64.StdEncoding.EncodeToString([]byte(subject)) + "?=\r\n")
	msg.WriteString("MIME-Version: 1.0\r\n")
	msg.WriteString("Content-Type: multipart/alternative; boundary=tw-boundary\r\n")
	msg.WriteString("\r\n--tw-boundary\r\n")
	msg.WriteString("Content-Type: text/plain; charset=UTF-8\r\n\r\n")
	msg.WriteString(textBody)
	msg.WriteString("\r\n--tw-boundary\r\n")
	msg.WriteString("Content-Type: text/html; charset=UTF-8\r\n\r\n")
	msg.WriteString(htmlBody)
	msg.WriteString("\r\n--tw-boundary--\r\n")

	auth := smtp.PlainAuth("", s.user, s.pass, s.host)
	addr := s.addr()

	if s.port == "465" {
		// 隐式 TLS（QQ/163 邮箱 465 端口）
		conn, err := tls.Dial("tcp", addr, &tls.Config{ServerName: s.host, MinVersion: tls.VersionTLS12})
		if err != nil {
			return fmt.Errorf("smtp tls dial: %w", err)
		}
		c, err := smtp.NewClient(conn, s.host)
		if err != nil {
			return fmt.Errorf("smtp client: %w", err)
		}
		defer c.Close()
		if err := c.Auth(auth); err != nil {
			return fmt.Errorf("smtp auth: %w", err)
		}
		return sendWithClient(c, s.plainFrom(), to, msg.String())
	}

	// 25/587: 明文连接后 STARTTLS
	c, err := smtp.Dial(addr)
	if err != nil {
		return fmt.Errorf("smtp dial: %w", err)
	}
	defer c.Close()
	if ok, _ := c.Extension("STARTTLS"); ok {
		if err := c.StartTLS(&tls.Config{ServerName: s.host, MinVersion: tls.VersionTLS12}); err != nil {
			return fmt.Errorf("smtp starttls: %w", err)
		}
	}
	if err := c.Auth(auth); err != nil {
		return fmt.Errorf("smtp auth: %w", err)
	}
	return sendWithClient(c, s.plainFrom(), to, msg.String())
}

func sendWithClient(c *smtp.Client, from, to, msg string) error {
	if err := c.Mail(from); err != nil {
		return fmt.Errorf("smtp mail: %w", err)
	}
	if err := c.Rcpt(to); err != nil {
		return fmt.Errorf("smtp rcpt: %w", err)
	}
	w, err := c.Data()
	if err != nil {
		return fmt.Errorf("smtp data: %w", err)
	}
	if _, err := w.Write([]byte(msg)); err != nil {
		return fmt.Errorf("smtp write: %w", err)
	}
	if err := w.Close(); err != nil {
		return fmt.Errorf("smtp data close: %w", err)
	}
	return c.Quit()
}

