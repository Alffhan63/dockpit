// Package notify sends alerts to Telegram, ntfy or a generic webhook.
package notify

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// Level ranks a message; channels show it as an emoji or priority.
type Level string

const (
	Info     Level = "info"
	Warning  Level = "warning"
	Critical Level = "critical"
	Recovery Level = "recovery"
)

// Message is one alert.
type Message struct {
	Title string `json:"title"`
	Body  string `json:"body"`
	Level Level  `json:"level"`
}

// Telegram sends through a bot.
type Telegram struct {
	BotToken string `json:"bot_token"`
	ChatID   string `json:"chat_id"`
}

// Ntfy publishes to an ntfy topic (ntfy.sh or self-hosted).
type Ntfy struct {
	Server string `json:"server"` // default https://ntfy.sh
	Topic  string `json:"topic"`
	Token  string `json:"token,omitempty"`
}

// Webhook POSTs the message as JSON.
type Webhook struct {
	URL string `json:"url"`
}

// Channels are the configured destinations. Empty ones are unused.
type Channels struct {
	Telegram Telegram `json:"telegram"`
	Ntfy     Ntfy     `json:"ntfy"`
	Webhook  Webhook  `json:"webhook"`
}

var client = &http.Client{Timeout: 10 * time.Second}

// Validate checks a configuration before it is saved.
func (c Channels) Validate() error {
	if (c.Telegram.BotToken == "") != (c.Telegram.ChatID == "") {
		return errors.New("telegram needs both bot token and chat id")
	}
	if c.Ntfy.Topic != "" {
		if strings.ContainsAny(c.Ntfy.Topic, "/ ?#") {
			return errors.New("ntfy topic contains invalid characters")
		}
		if c.Ntfy.Server != "" {
			if err := checkURL(c.Ntfy.Server); err != nil {
				return fmt.Errorf("ntfy server: %w", err)
			}
		}
	}
	if c.Webhook.URL != "" {
		if err := checkURL(c.Webhook.URL); err != nil {
			return fmt.Errorf("webhook: %w", err)
		}
	}
	return nil
}

func checkURL(s string) error {
	u, err := url.Parse(s)
	if err != nil || (u.Scheme != "https" && u.Scheme != "http") || u.Host == "" {
		return errors.New("must be an http(s) URL")
	}
	return nil
}

// Configured reports whether at least one channel is set up.
func (c Channels) Configured() bool {
	return c.Telegram.BotToken != "" || c.Ntfy.Topic != "" || c.Webhook.URL != ""
}

var emoji = map[Level]string{Info: "ℹ️", Warning: "⚠️", Critical: "🚨", Recovery: "✅"}

// Send delivers m to every configured channel and returns the joined errors
// of the ones that failed.
func (c Channels) Send(ctx context.Context, m Message) error {
	var errs []error
	if c.Telegram.BotToken != "" {
		text := emoji[m.Level] + " " + m.Title + "\n" + m.Body
		body, _ := json.Marshal(map[string]any{"chat_id": c.Telegram.ChatID, "text": text, "disable_web_page_preview": true})
		if err := post(ctx, "https://api.telegram.org/bot"+c.Telegram.BotToken+"/sendMessage", "application/json", body, nil); err != nil {
			errs = append(errs, fmt.Errorf("telegram: %w", redact(err, c.Telegram.BotToken)))
		}
	}
	if c.Ntfy.Topic != "" {
		server := strings.TrimRight(c.Ntfy.Server, "/")
		if server == "" {
			server = "https://ntfy.sh"
		}
		prio := map[Level]string{Info: "3", Warning: "4", Critical: "5", Recovery: "3"}[m.Level]
		hdr := map[string]string{"Title": m.Title, "Priority": prio, "Tags": map[Level]string{Info: "information_source", Warning: "warning", Critical: "rotating_light", Recovery: "white_check_mark"}[m.Level]}
		if c.Ntfy.Token != "" {
			hdr["Authorization"] = "Bearer " + c.Ntfy.Token
		}
		if err := post(ctx, server+"/"+c.Ntfy.Topic, "text/plain", []byte(m.Body), hdr); err != nil {
			errs = append(errs, fmt.Errorf("ntfy: %w", err))
		}
	}
	if c.Webhook.URL != "" {
		body, _ := json.Marshal(m)
		if err := post(ctx, c.Webhook.URL, "application/json", body, nil); err != nil {
			errs = append(errs, fmt.Errorf("webhook: %w", redact(err, c.Webhook.URL)))
		}
	}
	return errors.Join(errs...)
}

// header values must be ASCII-safe; ntfy titles with emoji go in the body.
func post(ctx context.Context, u, contentType string, body []byte, headers map[string]string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, u, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", contentType)
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode/100 != 2 {
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 200))
		return fmt.Errorf("HTTP %d %s", resp.StatusCode, strings.TrimSpace(string(b)))
	}
	return nil
}

// redact keeps secrets that appear in URLs out of logs and API errors.
func redact(err error, secret string) error {
	if secret == "" {
		return err
	}
	return errors.New(strings.ReplaceAll(err.Error(), secret, "***"))
}
