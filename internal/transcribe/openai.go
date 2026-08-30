package transcribe

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

var endpoint = "https://api.openai.com/v1/audio/transcriptions"

const modelEndpoint = "https://api.openai.com/v1/models/gpt-transcribe"

const model = "gpt-transcribe"

type Client interface {
	Transcribe(context.Context, string) (string, error)
}

func endpointForTest(value string) func() {
	old := endpoint
	endpoint = value
	return func() { endpoint = old }
}

type OpenAI struct {
	apiKey string
	http   *http.Client
}

func NewOpenAI(apiKey string) *OpenAI { return &OpenAI{apiKey: apiKey, http: http.DefaultClient} }

// Verify checks authentication and model visibility without uploading audio.
func Verify(ctx context.Context, apiKey string) error {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, modelEndpoint, nil)
	if err != nil {
		return err
	}
	request.Header.Set("Authorization", "Bearer "+apiKey)
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return fmt.Errorf("OpenAI returned %s", response.Status)
	}
	return nil
}

func (c *OpenAI) Transcribe(ctx context.Context, filename string) (string, error) {
	if strings.TrimSpace(c.apiKey) == "" {
		return "", errors.New("OpenAI API key is empty")
	}
	f, err := os.Open(filename)
	if err != nil {
		return "", err
	}
	defer f.Close()
	var body bytes.Buffer
	w := multipart.NewWriter(&body)
	part, err := w.CreateFormFile("file", filepath.Base(filename))
	if err != nil {
		return "", err
	}
	if _, err := io.Copy(part, f); err != nil {
		return "", err
	}
	if err := w.WriteField("model", model); err != nil {
		return "", err
	}
	if err := w.Close(); err != nil {
		return "", err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, &body)
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Bearer "+c.apiKey)
	req.Header.Set("Content-Type", w.FormDataContentType())
	resp, err := c.http.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	payload, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return "", err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("OpenAI transcription API returned %s: %s", resp.Status, strings.TrimSpace(string(payload)))
	}
	var data struct {
		Text string `json:"text"`
	}
	if err := json.Unmarshal(payload, &data); err != nil {
		return "", err
	}
	if strings.TrimSpace(data.Text) == "" {
		return "", errors.New("transcription response is empty")
	}
	return data.Text, nil
}
