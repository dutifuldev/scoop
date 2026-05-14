package translation

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
)

const (
	// DefaultLocalEndpoint points to a local OpenAI-compatible translation endpoint.
	DefaultLocalEndpoint = "http://127.0.0.1:8845/v1"
	// DefaultLocalModel is the default HY-MT model name.
	DefaultLocalModel = "tencent/HY-MT1.5-7B"
)

// LocalProvider translates text by calling an OpenAI-compatible chat completions endpoint.
type LocalProvider struct {
	endpointURL string
	model       string
	client      *http.Client
}

// NewLocalProviderFromEnv builds a local provider from env vars.
//   - TRANSLATION_ENDPOINT (default: http://127.0.0.1:8845/v1)
//   - TRANSLATION_MODEL (default: tencent/HY-MT1.5-7B)
func NewLocalProviderFromEnv() *LocalProvider {
	endpoint := strings.TrimSpace(os.Getenv("TRANSLATION_ENDPOINT"))
	if endpoint == "" {
		endpoint = DefaultLocalEndpoint
	}
	model := strings.TrimSpace(os.Getenv("TRANSLATION_MODEL"))
	if model == "" {
		model = DefaultLocalModel
	}
	return NewLocalProvider(endpoint, model)
}

// NewLocalProvider builds a local provider for the given endpoint/model.
func NewLocalProvider(endpoint, model string) *LocalProvider {
	normalizedEndpoint := normalizeEndpoint(endpoint)
	trimmedModel := strings.TrimSpace(model)
	if trimmedModel == "" {
		trimmedModel = DefaultLocalModel
	}
	return &LocalProvider{
		endpointURL: chatCompletionsURL(normalizedEndpoint),
		model:       trimmedModel,
		client: &http.Client{
			Timeout: 120 * time.Second,
		},
	}
}

func (p *LocalProvider) Name() string {
	return "local"
}

// ModelName returns the configured model identifier.
func (p *LocalProvider) ModelName() string {
	if p != nil {
		return p.model
	}
	return ""
}

func (p *LocalProvider) SupportedLanguages() []string {
	return SupportedTranslationLanguageCodes()
}

func (p *LocalProvider) Translate(ctx context.Context, req TranslateRequest) (*TranslateResponse, error) {
	normalized, err := normalizeTranslateRequest(req)
	if err != nil {
		return nil, err
	}

	started := time.Now()
	translated, err := p.sendTranslateRequest(ctx, normalized)
	if err != nil {
		return nil, err
	}

	latency := time.Since(started).Milliseconds()
	return &TranslateResponse{
		Text:         translated,
		SourceLang:   normalized.SourceLang,
		TargetLang:   normalized.TargetLang,
		ProviderName: p.Name(),
		LatencyMs:    latency,
	}, nil
}

func normalizeTranslateRequest(req TranslateRequest) (TranslateRequest, error) {
	text := strings.TrimSpace(req.Text)
	if text == "" {
		return TranslateRequest{}, fmt.Errorf("text is required")
	}
	targetLang := normalizeLangCode(req.TargetLang)
	if targetLang == "" {
		return TranslateRequest{}, fmt.Errorf("target language is required")
	}
	return TranslateRequest{
		Text:       text,
		SourceLang: normalizeLangCode(req.SourceLang),
		TargetLang: targetLang,
	}, nil
}

func (p *LocalProvider) sendTranslateRequest(ctx context.Context, req TranslateRequest) (string, error) {
	if p == nil {
		return "", fmt.Errorf("local provider is nil")
	}
	body, err := json.Marshal(p.localChatRequest(req))
	if err != nil {
		return "", fmt.Errorf("marshal translation request: %w", err)
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, p.endpointURL, bytes.NewReader(body))
	if err != nil {
		return "", fmt.Errorf("build translation request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := p.client.Do(httpReq)
	if err != nil {
		return "", fmt.Errorf("send translation request: %w", err)
	}
	defer resp.Body.Close()
	return parseLocalChatHTTPResponse(resp)
}

func (p *LocalProvider) localChatRequest(req TranslateRequest) localChatRequest {
	return localChatRequest{
		Model: p.model,
		Messages: []localChatMessage{{
			Role:    "user",
			Content: buildHYMTPrompt(req.Text, req.SourceLang, req.TargetLang),
		}},
		Temperature: 0.7,
		TopP:        0.6,
	}
}

func parseLocalChatHTTPResponse(resp *http.Response) (string, error) {
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("read translation response: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", localChatStatusError(resp.StatusCode, respBody)
	}
	return translatedTextFromLocalChatResponse(respBody)
}

func localChatStatusError(statusCode int, respBody []byte) error {
	var errPayload localChatErrorResponse
	if unmarshalErr := json.Unmarshal(respBody, &errPayload); unmarshalErr == nil {
		if msg := strings.TrimSpace(errPayload.Error.Message); msg != "" {
			return fmt.Errorf("translation endpoint status %d: %s", statusCode, msg)
		}
	}
	return fmt.Errorf("translation endpoint status %d: %s", statusCode, strings.TrimSpace(string(respBody)))
}

func translatedTextFromLocalChatResponse(respBody []byte) (string, error) {
	var parsed localChatResponse
	if err := json.Unmarshal(respBody, &parsed); err != nil {
		return "", fmt.Errorf("decode translation response: %w", err)
	}
	if len(parsed.Choices) == 0 {
		return "", fmt.Errorf("translation response missing choices")
	}
	translated := strings.TrimSpace(parsed.Choices[0].Message.Content)
	if translated == "" {
		return "", fmt.Errorf("translation response was empty")
	}
	return translated, nil
}

type localChatRequest struct {
	Model       string             `json:"model"`
	Messages    []localChatMessage `json:"messages"`
	Temperature float64            `json:"temperature,omitempty"`
	TopP        float64            `json:"top_p,omitempty"`
}

type localChatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type localChatResponse struct {
	Choices []struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
	} `json:"choices"`
}

type localChatErrorResponse struct {
	Error struct {
		Message string `json:"message"`
	} `json:"error"`
}

func buildHYMTPrompt(text, sourceLang, targetLang string) string {
	target := targetLanguageLabel(targetLang)
	if isChineseLanguage(sourceLang) || isChineseLanguage(targetLang) {
		// HY-MT zh<=>xx template.
		return fmt.Sprintf("将以下文本翻译为%s，注意只需要输出翻译后的结果，不要额外解释：\n\n%s", target.chinese, text)
	}
	// HY-MT xx<=>xx template.
	return fmt.Sprintf("Translate the following segment into %s, without additional explanation.\n\n%s", target.english, text)
}

func targetLanguageLabel(lang string) languageLabel {
	normalized := normalizeLangCode(lang)
	if labels, ok := translationLanguageLabels[normalized]; ok {
		return labels
	}
	fallback := strings.TrimSpace(lang)
	if fallback == "" {
		fallback = "English"
	}
	return languageLabel{english: fallback, chinese: fallback}
}

func isChineseLanguage(lang string) bool {
	return normalizeLangCode(lang) == "zh"
}

func normalizeEndpoint(raw string) string {
	endpoint := strings.TrimSpace(raw)
	if endpoint == "" {
		return DefaultLocalEndpoint
	}
	if !strings.Contains(endpoint, "://") {
		endpoint = "http://" + endpoint
	}

	parsed, err := url.Parse(endpoint)
	if err != nil || strings.TrimSpace(parsed.Host) == "" {
		return DefaultLocalEndpoint
	}
	parsed.Path = strings.TrimRight(parsed.Path, "/")
	if parsed.Path == "" {
		parsed.Path = "/v1"
	}
	return parsed.String()
}

func chatCompletionsURL(endpoint string) string {
	parsed, err := url.Parse(endpoint)
	if err != nil || strings.TrimSpace(parsed.Host) == "" {
		return DefaultLocalEndpoint + "/chat/completions"
	}

	path := strings.TrimRight(parsed.Path, "/")
	switch {
	case strings.HasSuffix(path, "/chat/completions"):
		parsed.Path = path
	case strings.HasSuffix(path, "/v1"):
		parsed.Path = path + "/chat/completions"
	case path == "":
		parsed.Path = "/v1/chat/completions"
	default:
		parsed.Path = path + "/v1/chat/completions"
	}

	return parsed.String()
}
