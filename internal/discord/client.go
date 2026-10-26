// Package discord delivers artifacts to one configured private DM recipient.
package discord

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

const maxUpload = 10 << 20 // Conservative per-message budget; Discord can impose lower limits.
var snowflake = regexp.MustCompile(`^[0-9]{1,20}$`)

type Client struct {
	token, recipient, baseURL string
	http                      *http.Client
}

func New(token, recipient string) (*Client, error) {
	token, recipient = strings.TrimSpace(token), strings.TrimSpace(recipient)
	if token == "" || !snowflake.MatchString(recipient) || strings.Trim(recipient, "0") == "" {
		return nil, fmt.Errorf("Discord delivery requires EXAMGEN_DISCORD_BOT_TOKEN and a numeric EXAMGEN_DISCORD_USER_ID")
	}
	return &Client{token, recipient, "https://discord.com/api/v10", &http.Client{Timeout: 60 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}}, nil
}

// Send uploads only the explicitly supplied artifacts. Empty paths sends a notification.
func (c *Client) Send(ctx context.Context, content string, paths ...string) error {
	if len(paths) > 2 {
		return fmt.Errorf("at most two attachments are supported")
	}
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	payload, err := json.Marshal(map[string]any{"content": content, "allowed_mentions": map[string]any{"parse": []string{}}})
	if err != nil {
		return err
	}
	if err := writer.WriteField("payload_json", string(payload)); err != nil {
		return err
	}
	for i, path := range paths {
		if err := addFile(writer, i, path, maxUpload-int64(body.Len())); err != nil {
			return err
		}
	}
	if err := writer.Close(); err != nil {
		return err
	}
	if body.Len() > maxUpload {
		return fmt.Errorf("Discord attachments exceed the 10 MiB upload budget; files remain local")
	}
	dmPayload, _ := json.Marshal(map[string]string{"recipient_id": c.recipient})
	var channel struct {
		ID string `json:"id"`
	}
	if err := c.post(ctx, "/users/@me/channels", "application/json", dmPayload, &channel); err != nil {
		return fmt.Errorf("open private DM: %w", err)
	}
	if !snowflake.MatchString(channel.ID) {
		return fmt.Errorf("Discord returned an invalid DM channel ID")
	}
	if err := c.post(ctx, "/channels/"+channel.ID+"/messages", writer.FormDataContentType(), body.Bytes(), nil); err != nil {
		return fmt.Errorf("send private DM: %w", err)
	}
	return nil
}

func addFile(w *multipart.Writer, index int, path string, remaining int64) error {
	f, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("open attachment: %w", err)
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() || info.Size() == 0 {
		return fmt.Errorf("attachment must be a nonempty regular file: %s", path)
	}
	if info.Size() > remaining {
		return fmt.Errorf("Discord attachments exceed the 10 MiB upload budget; files remain local")
	}
	part, err := w.CreateFormFile(fmt.Sprintf("files[%d]", index), filepath.Base(path))
	if err != nil {
		return err
	}
	n, err := io.Copy(part, io.LimitReader(f, remaining+1))
	if err != nil {
		return err
	}
	if n > remaining {
		return fmt.Errorf("attachment grew beyond upload budget")
	}
	return nil
}

func (c *Client) post(ctx context.Context, path, contentType string, body []byte, result any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+path, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bot "+c.token)
	req.Header.Set("Content-Type", contentType)
	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("Discord request failed (check connectivity or timeout)")
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		// Do not echo remote response bodies: they may contain request data or credentials.
		switch resp.StatusCode {
		case 401:
			return fmt.Errorf("Discord rejected the bot token (HTTP 401)")
		case 403:
			return fmt.Errorf("Discord blocked delivery (HTTP 403); check DM privacy settings and bot access")
		case 413:
			return fmt.Errorf("Discord rejected the attachment size (HTTP 413)")
		case 429:
			return fmt.Errorf("Discord rate limited delivery (HTTP 429); retry later using the deliver command")
		default:
			return fmt.Errorf("Discord returned HTTP %d", resp.StatusCode)
		}
	}
	if result != nil {
		if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(result); err != nil {
			return fmt.Errorf("invalid Discord response")
		}
	}
	return nil
}
