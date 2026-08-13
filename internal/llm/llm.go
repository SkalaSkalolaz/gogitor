package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"strings"
	"time"
    "bufio"

	"gogitor/internal/textutil"
	"gogitor/internal/config"
)

type Client struct {
	cfg  *config.Config
	http *http.Client
	log  *slog.Logger
}

func NewClient(cfg *config.Config, log *slog.Logger) *Client {
	timeout := time.Duration(cfg.LLMTimeout) * time.Second
	if timeout <= 0 {
		timeout = 300 * time.Second
	}

	return &Client{
		cfg: cfg,
		http: &http.Client{
			Timeout: timeout,
		},
		log: log,
	}
}

func (c *Client) Send(ctx context.Context, prompt string) (string, error) {
	provider := strings.ToLower(strings.TrimSpace(c.cfg.Provider))

	if base, ok := config.OpenAIBaseFromProvider(c.cfg.Provider); ok {
		return c.sendOpenAICompatible(ctx, base, prompt)
	}

	switch {
	case provider == "ollama":
		base := c.cfg.OllamaURL
		if base == "" {
			base = os.Getenv("OLLAMA_HOST")
		}
		if base == "" {
			base = "http://localhost:11434"
		}

		return c.sendOllama(ctx, base, prompt)

	case strings.HasPrefix(provider, "http://") || strings.HasPrefix(provider, "https://"):
		return c.sendOllama(ctx, provider, prompt)

	default:
		return "", fmt.Errorf(
			"unsupported provider %q; supported: ollama or http(s) URL",
			c.cfg.Provider,
		)
	}
}

func (c *Client) sendOllama(ctx context.Context, baseURL, prompt string) (string, error) {
    endpoint := strings.TrimRight(baseURL, "/") + "/api/generate"

    estimatedTokens := (len(prompt) + 3) / 4
    
    // Запас на генерацию ответа масштабируется от размера контекста:
    // для малых моделей — 2048, для больших — до 16384
    responseReserve := 2048
    maxCtx := c.cfg.EffectiveContextTokens()
    if maxCtx > 65536 {
        responseReserve = 8192
    }
    if maxCtx > 131072 {
        responseReserve = 16384
    }

    numCtx := estimatedTokens + responseReserve

    // Минимум не меняем
    if numCtx < 4096 {
        numCtx = 4096
    }
    // Потолок теперь конфигурируемый
    if numCtx > maxCtx {
        numCtx = maxCtx
    }

    payload := map[string]any{
        "model":  c.cfg.Model,
        "prompt": prompt,
        "stream": false,
        "options": map[string]any{
            "num_ctx": numCtx,
        },
    }

    body, status, err := c.postJSON(ctx, endpoint, payload, nil)
	if err != nil {
		return "", err
	}

	if status != http.StatusOK {
		return "", fmt.Errorf("ollama HTTP %d: %s", status, errorSnippet(body))
	}

	var resp struct {
		Response string `json:"response"`
		Error    string `json:"error"`
	}

	if err := json.Unmarshal(body, &resp); err != nil {
		return "", fmt.Errorf("invalid ollama response: %w", err)
	}

	if resp.Error != "" {
		return "", fmt.Errorf("ollama error: %s", resp.Error)
	}

	return strings.TrimSpace(resp.Response), nil
}

func (c *Client) postJSON(ctx context.Context, endpoint string, payload any, headers map[string]string) ([]byte, int, error) {
	data, err := json.Marshal(payload)
	if err != nil {
		return nil, 0, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(data))
	if err != nil {
		return nil, 0, err
	}

	req.Header.Set("Content-Type", "application/json")

	for k, v := range headers {
		req.Header.Set(k, v)
	}

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, 0, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 10<<20))
	if err != nil {
		return nil, resp.StatusCode, err
	}

	return body, resp.StatusCode, nil
}

func errorSnippet(body []byte) string {
	s := strings.TrimSpace(string(body))
	return textutil.LimitRunes(s, 700, "...")
}

func (c *Client) sendOpenAICompatible(ctx context.Context, baseURL, prompt string) (string, error) {
	endpoint := openAIChatEndpoint(baseURL)

    maxTokens := 4096
    if c.cfg.EffectiveContextTokens() > 65536 {
        maxTokens = 16384
    }
    if c.cfg.EffectiveContextTokens() > 131072 {
        maxTokens = 32768
    }


	payload := map[string]any{
		"model": c.cfg.Model,
		"messages": []map[string]string{
			{
				"role":    "user",
				"content": prompt,
			},
		},
		"stream": false,
		"max_tokens": maxTokens,
	}



	headers := map[string]string{}

	if key := strings.TrimSpace(c.cfg.APIKey); key != "" {
		headers["Authorization"] = "Bearer " + key
	}

	body, status, err := c.postJSON(ctx, endpoint, payload, headers)
	if err != nil {
		return "", err
	}

	if status != http.StatusOK {
		return "", fmt.Errorf("openai-compatible HTTP %d: %s", status, errorSnippet(body))
	}

	content := parseOpenAIContent(body)
	if content == "" {
		return "", fmt.Errorf("openai-compatible returned empty content: %s", errorSnippet(body))
	}

	return content, nil
}

func openAIChatEndpoint(baseURL string) string {
	base := strings.TrimRight(baseURL, "/")
	lower := strings.ToLower(base)

	if strings.HasSuffix(lower, "/chat/completions") {
		return base
	}

	if strings.HasSuffix(lower, "/v1") {
		return base + "/chat/completions"
	}

	return base + "/v1/chat/completions"
}

func parseOpenAIContent(body []byte) string {
	var resp struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
			Text string `json:"text"`
		} `json:"choices"`
	}

	if err := json.Unmarshal(body, &resp); err != nil {
		return ""
	}

	if len(resp.Choices) == 0 {
		return ""
	}

	if resp.Choices[0].Message.Content != "" {
		return strings.TrimSpace(resp.Choices[0].Message.Content)
	}

	return strings.TrimSpace(resp.Choices[0].Text)
}

// Stream выполняет LLM-запрос и вызывает onToken для каждого полученного токена.
// Возвращает полный итоговый текст.
func (c *Client) Stream(ctx context.Context, prompt string, onToken func(string)) (string, error) {
	if onToken == nil {
		return c.Send(ctx, prompt)
	}

	provider := strings.ToLower(strings.TrimSpace(c.cfg.Provider))

	if base, ok := config.OpenAIBaseFromProvider(c.cfg.Provider); ok {
		return c.streamOpenAICompatible(ctx, base, prompt, onToken)
	}

	switch {
	case provider == "ollama":
		base := c.cfg.OllamaURL
		if base == "" {
			base = os.Getenv("OLLAMA_HOST")
		}
		if base == "" {
			base = "http://localhost:11434"
		}
		return c.streamOllama(ctx, base, prompt, onToken)

	case strings.HasPrefix(provider, "http://") || strings.HasPrefix(provider, "https://"):
		return c.streamOllama(ctx, provider, prompt, onToken)

	default:
		// Если провайдер не поддерживает стриминг — деградируем до Send.
		return c.Send(ctx, prompt)
	}
}

func (c *Client) streamHTTP() *http.Client {
	// Для стриминга нельзя использовать общий client.Timeout,
	// иначе долгая генерация оборвётся по таймауту.
	return &http.Client{
		Transport: c.http.Transport,
	}
}

func (c *Client) streamOllama(
	ctx context.Context,
	baseURL, prompt string,
	onToken func(string),
) (string, error) {
	endpoint := strings.TrimRight(baseURL, "/") + "/api/generate"

	estimatedTokens := (len(prompt) + 3) / 4

	responseReserve := 2048
	maxCtx := c.cfg.EffectiveContextTokens()
	if maxCtx > 65536 {
		responseReserve = 8192
	}
	if maxCtx > 131072 {
		responseReserve = 16384
	}

	numCtx := estimatedTokens + responseReserve
	if numCtx < 4096 {
		numCtx = 4096
	}
	if numCtx > maxCtx {
		numCtx = maxCtx
	}

	payload := map[string]any{
		"model":  c.cfg.Model,
		"prompt": prompt,
		"stream": true,
		"options": map[string]any{
			"num_ctx": numCtx,
		},
	}

	data, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(data))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.streamHTTP().Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
		return "", fmt.Errorf("ollama stream HTTP %d: %s", resp.StatusCode, errorSnippet(body))
	}

	var full strings.Builder
	scanner := bufio.NewScanner(resp.Body)
	scanner.Buffer(make([]byte, 64*1024), 4*1024*1024)

	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}

		var chunk struct {
			Response string `json:"response"`
			Error    string `json:"error"`
			Done     bool   `json:"done"`
		}

		if err := json.Unmarshal(line, &chunk); err != nil {
			// Ollama обычно шлёт NDJSON, но пропускаем мусорные строки.
			continue
		}

		if chunk.Error != "" {
			return full.String(), fmt.Errorf("ollama stream error: %s", chunk.Error)
		}

		if chunk.Response != "" {
			full.WriteString(chunk.Response)
			if onToken != nil {
				onToken(chunk.Response)
			}
		}

		if chunk.Done {
			break
		}
	}

	if err := scanner.Err(); err != nil {
		return full.String(), err
	}

	return strings.TrimSpace(full.String()), nil
}

func (c *Client) streamOpenAICompatible(
	ctx context.Context,
	baseURL, prompt string,
	onToken func(string),
) (string, error) {
	endpoint := openAIChatEndpoint(baseURL)

	maxTokens := 4096
	if c.cfg.EffectiveContextTokens() > 65536 {
		maxTokens = 16384
	}
	if c.cfg.EffectiveContextTokens() > 131072 {
		maxTokens = 32768
	}

	payload := map[string]any{
		"model": c.cfg.Model,
		"messages": []map[string]string{
			{
				"role":    "user",
				"content": prompt,
			},
		},
		"stream":     true,
		"max_tokens": maxTokens,
	}

	headers := map[string]string{}
	if key := strings.TrimSpace(c.cfg.APIKey); key != "" {
		headers["Authorization"] = "Bearer " + key
	}

	data, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(data))
	if err != nil {
		return "", err
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "text/event-stream")
	for k, v := range headers {
		req.Header.Set(k, v)
	}

	resp, err := c.streamHTTP().Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
		return "", fmt.Errorf("openai-compatible stream HTTP %d: %s", resp.StatusCode, errorSnippet(body))
	}

	var full strings.Builder
	reader := bufio.NewReader(resp.Body)

	for {
		line, readErr := reader.ReadString('\n')
		line = strings.TrimSpace(line)

		if line != "" {
			if strings.HasPrefix(line, "data:") {
				dataPart := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
				if dataPart == "[DONE]" {
					break
				}

				if content := parseOpenAIStreamChunk([]byte(dataPart)); content != "" {
					full.WriteString(content)
					if onToken != nil {
						onToken(content)
					}
				}
			}
		}

		if readErr != nil {
			if readErr == io.EOF {
				break
			}
			return full.String(), readErr
		}
	}

	return strings.TrimSpace(full.String()), nil
}

func parseOpenAIStreamChunk(data []byte) string {
	var resp struct {
		Choices []struct {
			Delta struct {
				Content string `json:"content"`
			} `json:"delta"`
			Text string `json:"text"`
		} `json:"choices"`
	}

	if err := json.Unmarshal(data, &resp); err != nil {
		return ""
	}

	if len(resp.Choices) == 0 {
		return ""
	}

	if resp.Choices[0].Delta.Content != "" {
		return resp.Choices[0].Delta.Content
	}

	return resp.Choices[0].Text
}