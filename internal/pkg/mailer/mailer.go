package mailer

import (
	"crypto/tls"
	"encoding/base64"
	"errors"
	"fmt"
	"net"
	"net/smtp"
	"strconv"
	"strings"
	"time"

	"github.com/UtopiaXC/UtopiaImages/internal/config"
)

// loginAuth implements the SMTP LOGIN authentication mechanism (used by Office 365, Exchange, 163, etc.)
type loginAuth struct {
	username, password string
	step               int
}

func LoginAuth(username, password string) smtp.Auth {
	return &loginAuth{username: username, password: password}
}

func (a *loginAuth) Start(server *smtp.ServerInfo) (string, []byte, error) {
	a.step = 0
	return "LOGIN", []byte{}, nil
}

func (a *loginAuth) Next(fromServer []byte, more bool) ([]byte, error) {
	if more {
		a.step++
		if a.step == 1 {
			return []byte(a.username), nil
		}
		if a.step == 2 {
			return []byte(a.password), nil
		}
		return nil, fmt.Errorf("unexpected server challenge: %s", string(fromServer))
	}
	return nil, nil
}

// SendMail sends an email with full compatibility for direct SSL (port 465), STARTTLS (port 587/25), and both AUTH LOGIN / AUTH PLAIN.
func SendMail(cfg config.SMTPConfig, to, subject, body string) error {
	if strings.Contains(body, "<html") || strings.Contains(body, "<!DOCTYPE") {
		return SendHTMLEmail(cfg, to, subject, body, "")
	}
	return sendRawMail(cfg, to, subject, body, false)
}

// SendHTMLEmail sends a multipart/alternative email with modern HTML and text fallback.
func SendHTMLEmail(cfg config.SMTPConfig, to, subject, htmlBody, textBody string) error {
	if textBody == "" {
		textBody = "Please view this email in an email client that supports HTML."
	}
	if !cfg.Enabled || cfg.Host == "" {
		return errors.New("smtp is not configured")
	}

	addr := net.JoinHostPort(cfg.Host, strconv.Itoa(cfg.Port))
	from := cfg.From
	if from == "" {
		from = cfg.User
	}
	fromName := cfg.FromName
	if fromName == "" {
		fromName = "Utopia Images"
	}

	headerSanitizer := strings.NewReplacer("\r", "", "\n", "")
	cleanFrom := headerSanitizer.Replace(from)
	cleanFromName := headerSanitizer.Replace(fromName)
	cleanTo := headerSanitizer.Replace(to)

	encodedSubject := fmt.Sprintf("=?UTF-8?B?%s?=", base64.StdEncoding.EncodeToString([]byte(headerSanitizer.Replace(subject))))
	boundary := fmt.Sprintf("=_part_%d", time.Now().UnixNano())

	msg := fmt.Sprintf("From: %s <%s>\r\nTo: %s\r\nSubject: %s\r\nMIME-Version: 1.0\r\nContent-Type: multipart/alternative; boundary=\"%s\"\r\n\r\n--%s\r\nContent-Type: text/plain; charset=UTF-8\r\nContent-Transfer-Encoding: base64\r\n\r\n%s\r\n\r\n--%s\r\nContent-Type: text/html; charset=UTF-8\r\nContent-Transfer-Encoding: base64\r\n\r\n%s\r\n\r\n--%s--",
		cleanFromName, cleanFrom, cleanTo, encodedSubject, boundary,
		boundary, base64.StdEncoding.EncodeToString([]byte(textBody)),
		boundary, base64.StdEncoding.EncodeToString([]byte(htmlBody)),
		boundary)

	client, err := createSMTPClient(cfg, addr)
	if err != nil {
		return err
	}
	defer client.Quit()

	if err := authenticateClient(client, cfg); err != nil {
		return err
	}

	if err := client.Mail(from); err != nil {
		return fmt.Errorf("smtp mail from failed: %w", err)
	}
	if err := client.Rcpt(to); err != nil {
		return fmt.Errorf("smtp rcpt to failed: %w", err)
	}
	w, err := client.Data()
	if err != nil {
		return fmt.Errorf("smtp data failed: %w", err)
	}
	if _, err := w.Write([]byte(msg)); err != nil {
		return fmt.Errorf("smtp write body failed: %w", err)
	}
	if err := w.Close(); err != nil {
		return fmt.Errorf("smtp data close failed: %w", err)
	}

	return nil
}

func sendRawMail(cfg config.SMTPConfig, to, subject, body string, isHTML bool) error {
	if !cfg.Enabled || cfg.Host == "" {
		return errors.New("smtp is not configured")
	}

	addr := net.JoinHostPort(cfg.Host, strconv.Itoa(cfg.Port))
	from := cfg.From
	if from == "" {
		from = cfg.User
	}
	fromName := cfg.FromName
	if fromName == "" {
		fromName = "Utopia Images"
	}

	headerSanitizer := strings.NewReplacer("\r", "", "\n", "")
	cleanFrom := headerSanitizer.Replace(from)
	cleanFromName := headerSanitizer.Replace(fromName)
	cleanTo := headerSanitizer.Replace(to)

	encodedSubject := fmt.Sprintf("=?UTF-8?B?%s?=", base64.StdEncoding.EncodeToString([]byte(headerSanitizer.Replace(subject))))
	contentType := "text/plain; charset=UTF-8"
	if isHTML {
		contentType = "text/html; charset=UTF-8"
	}
	msg := fmt.Sprintf("From: %s <%s>\r\nTo: %s\r\nSubject: %s\r\nContent-Type: %s\r\n\r\n%s",
		cleanFromName, cleanFrom, cleanTo, encodedSubject, contentType, body)

	client, err := createSMTPClient(cfg, addr)
	if err != nil {
		return err
	}
	defer client.Quit()

	if err := authenticateClient(client, cfg); err != nil {
		return err
	}

	if err := client.Mail(from); err != nil {
		return fmt.Errorf("smtp mail from failed: %w", err)
	}
	if err := client.Rcpt(to); err != nil {
		return fmt.Errorf("smtp rcpt to failed: %w", err)
	}
	w, err := client.Data()
	if err != nil {
		return fmt.Errorf("smtp data failed: %w", err)
	}
	if _, err := w.Write([]byte(msg)); err != nil {
		return fmt.Errorf("smtp write body failed: %w", err)
	}
	if err := w.Close(); err != nil {
		return fmt.Errorf("smtp data close failed: %w", err)
	}

	return nil
}

// TestConnection verifies TCP connection, TLS negotiation, and SMTP authentication against the target server.
func TestConnection(cfg config.SMTPConfig) error {
	addr := net.JoinHostPort(cfg.Host, strconv.Itoa(cfg.Port))
	client, err := createSMTPClient(cfg, addr)
	if err != nil {
		return err
	}
	defer client.Quit()

	if cfg.User != "" && cfg.Password != "" {
		if err := authenticateClient(client, cfg); err != nil {
			return err
		}
	}

	return nil
}

func createSMTPClient(cfg config.SMTPConfig, addr string) (*smtp.Client, error) {
	enc := strings.ToLower(cfg.Encryption)
	// Port 465 is always implicit direct SSL. Port 587 and 25 must use STARTTLS.
	useDirectTLS := (enc == "ssl" && cfg.Port != 587 && cfg.Port != 25) || cfg.Port == 465

	dialer := &net.Dialer{Timeout: 10 * time.Second}
	tlsConfig := &tls.Config{
		ServerName: cfg.Host,
	}

	if useDirectTLS {
		conn, err := tls.DialWithDialer(dialer, "tcp", addr, tlsConfig)
		if err != nil {
			return nil, fmt.Errorf("tls connection failed: %w", err)
		}
		client, err := smtp.NewClient(conn, cfg.Host)
		if err != nil {
			_ = conn.Close()
			return nil, fmt.Errorf("smtp client initialization failed: %w", err)
		}
		return client, nil
	}

	conn, err := dialer.Dial("tcp", addr)
	if err != nil {
		return nil, fmt.Errorf("tcp connection failed: %w", err)
	}
	client, err := smtp.NewClient(conn, cfg.Host)
	if err != nil {
		_ = conn.Close()
		return nil, fmt.Errorf("smtp client initialization failed: %w", err)
	}

	// Upgrade via STARTTLS if supported or requested
	if ok, _ := client.Extension("STARTTLS"); ok || enc == "tls" || enc == "starttls" || (enc == "ssl" && cfg.Port == 587) {
		if err := client.StartTLS(tlsConfig); err != nil {
			_ = client.Close()
			return nil, fmt.Errorf("starttls negotiation failed: %w", err)
		}
	}

	return client, nil
}

func authenticateClient(client *smtp.Client, cfg config.SMTPConfig) error {
	if cfg.User == "" || cfg.Password == "" {
		return nil
	}

	ok, authTypes := client.Extension("AUTH")
	upperAuth := strings.ToUpper(authTypes)

	// If server advertises LOGIN (like Office 365, Exchange, 163, QQ), prioritize LOGIN auth
	if ok && strings.Contains(upperAuth, "LOGIN") {
		auth := LoginAuth(cfg.User, cfg.Password)
		if err := client.Auth(auth); err != nil {
			return fmt.Errorf("smtp LOGIN auth failed: %w", err)
		}
		return nil
	}

	// If server advertises PLAIN, use PlainAuth
	if ok && strings.Contains(upperAuth, "PLAIN") {
		auth := smtp.PlainAuth("", cfg.User, cfg.Password, cfg.Host)
		if err := client.Auth(auth); err != nil {
			return fmt.Errorf("smtp PLAIN auth failed: %w", err)
		}
		return nil
	}

	// Fallback: try LOGIN first, then PLAIN
	loginErr := client.Auth(LoginAuth(cfg.User, cfg.Password))
	if loginErr == nil {
		return nil
	}

	plainErr := client.Auth(smtp.PlainAuth("", cfg.User, cfg.Password, cfg.Host))
	if plainErr == nil {
		return nil
	}

	return fmt.Errorf("smtp authentication failed: login error: %v, plain error: %v", loginErr, plainErr)
}
