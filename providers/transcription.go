// 语音转写提供商实现。
// 提供 TranscriptionProvider 接口和 Groq Whisper API 的具体实现。

package providers

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
	"time"
)

// TranscriptionProvider 是语音转写的接口。
type TranscriptionProvider interface {
	// Transcribe 将音频文件转写为文本。
	Transcribe(ctx context.Context, filePath string) (string, error)
}

// GroqTranscriptionProvider 使用 Groq 的 Whisper API 提供语音转写。
type GroqTranscriptionProvider struct {
	apiKey string       // API 密钥
	apiURL string       // API 端点 URL
	client *http.Client // HTTP 客户端
}

// NewGroqTranscriptionProvider 创建一个新的 Groq 转写提供商。
// 如果 apiKey 为空，则尝试从 GROQ_API_KEY 环境变量获取。
func NewGroqTranscriptionProvider(apiKey string) *GroqTranscriptionProvider {
	if apiKey == "" {
		apiKey = os.Getenv("GROQ_API_KEY")
	}
	return &GroqTranscriptionProvider{
		apiKey: apiKey,
		apiURL: "https://api.groq.com/openai/v1/audio/transcriptions",
		client: &http.Client{Timeout: 60 * time.Second},
	}
}

// Transcribe 使用 Groq 的 Whisper API 转写音频文件。
// 通过 multipart/form-data 上传音频文件，使用 whisper-large-v3 模型。
func (g *GroqTranscriptionProvider) Transcribe(ctx context.Context, filePath string) (string, error) {
	if g.apiKey == "" {
		return "", fmt.Errorf("groq API key not configured")
	}

	f, err := os.Open(filePath)
	if err != nil {
		return "", fmt.Errorf("open audio file: %w", err)
	}
	defer f.Close()

	var buf bytes.Buffer
	writer := multipart.NewWriter(&buf)

	// 添加音频文件
	part, err := writer.CreateFormFile("file", filepath.Base(filePath))
	if err != nil {
		return "", fmt.Errorf("create form file: %w", err)
	}
	if _, err := io.Copy(part, f); err != nil {
		return "", fmt.Errorf("copy file: %w", err)
	}

	// 添加模型参数
	if err := writer.WriteField("model", "whisper-large-v3"); err != nil {
		return "", fmt.Errorf("write model field: %w", err)
	}
	writer.Close()

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, g.apiURL, &buf)
	if err != nil {
		return "", fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+g.apiKey)
	req.Header.Set("Content-Type", writer.FormDataContentType())

	resp, err := g.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("http request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("groq API error (status %d): %s", resp.StatusCode, string(body))
	}

	var result struct {
		Text string `json:"text"` // 转写结果文本
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return "", fmt.Errorf("parse response: %w", err)
	}

	return result.Text, nil
}
