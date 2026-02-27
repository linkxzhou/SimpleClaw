package tools

import (
	"context"
	"errors"
	"strings"
	"testing"
)

func TestMessageTool_Name(t *testing.T) {
	tool := NewMessageTool(nil)
	if tool.Name() != "message" {
		t.Errorf("Name = %q, want %q", tool.Name(), "message")
	}
}

func TestMessageTool_Description(t *testing.T) {
	tool := NewMessageTool(nil)
	if tool.Description() == "" {
		t.Error("Description should not be empty")
	}
}

func TestMessageTool_Parameters(t *testing.T) {
	tool := NewMessageTool(nil)
	params := tool.Parameters()
	if params["type"] != "object" {
		t.Errorf("Parameters type = %v, want object", params["type"])
	}
}

func TestMessageTool_SendSuccess(t *testing.T) {
	var sent OutboundMessage
	callback := func(msg OutboundMessage) error {
		sent = msg
		return nil
	}

	tool := NewMessageTool(callback)
	tool.SetContext("telegram", "12345")

	result, err := tool.Execute(context.Background(), map[string]interface{}{
		"content": "hello",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(result, "Message sent") {
		t.Errorf("result = %q, expected sent confirmation", result)
	}
	if sent.Channel != "telegram" || sent.ChatID != "12345" || sent.Content != "hello" {
		t.Errorf("sent = %+v, unexpected", sent)
	}
}

func TestMessageTool_CustomChannelOverride(t *testing.T) {
	var sent OutboundMessage
	callback := func(msg OutboundMessage) error {
		sent = msg
		return nil
	}

	tool := NewMessageTool(callback)
	tool.SetContext("telegram", "111")

	result, err := tool.Execute(context.Background(), map[string]interface{}{
		"content": "hello",
		"channel": "discord",
		"chat_id": "222",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(result, "Message sent to discord:222") {
		t.Errorf("result = %q, expected discord:222", result)
	}
	if sent.Channel != "discord" || sent.ChatID != "222" {
		t.Errorf("sent = %+v, expected discord:222", sent)
	}
}

func TestMessageTool_EmptyContent(t *testing.T) {
	tool := NewMessageTool(nil)
	_, err := tool.Execute(context.Background(), map[string]interface{}{
		"content": "",
	})
	if err == nil {
		t.Fatal("expected error for empty content")
	}
}

func TestMessageTool_NoContext(t *testing.T) {
	callback := func(msg OutboundMessage) error { return nil }
	tool := NewMessageTool(callback)

	result, err := tool.Execute(context.Background(), map[string]interface{}{
		"content": "hello",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(result, "No target channel") {
		t.Errorf("result = %q, expected no-target error", result)
	}
}

func TestMessageTool_NilCallback(t *testing.T) {
	tool := NewMessageTool(nil)
	tool.SetContext("telegram", "12345")

	result, err := tool.Execute(context.Background(), map[string]interface{}{
		"content": "hello",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(result, "not configured") {
		t.Errorf("result = %q, expected not-configured error", result)
	}
}

func TestMessageTool_CallbackError(t *testing.T) {
	callback := func(msg OutboundMessage) error {
		return errors.New("network error")
	}

	tool := NewMessageTool(callback)
	tool.SetContext("telegram", "12345")

	result, err := tool.Execute(context.Background(), map[string]interface{}{
		"content": "hello",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(result, "network error") {
		t.Errorf("result = %q, expected callback error", result)
	}
}

func TestMessageTool_SetContextUpdates(t *testing.T) {
	var sent OutboundMessage
	callback := func(msg OutboundMessage) error {
		sent = msg
		return nil
	}

	tool := NewMessageTool(callback)
	tool.SetContext("telegram", "111")
	tool.SetContext("discord", "222")

	_, err := tool.Execute(context.Background(), map[string]interface{}{
		"content": "test",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if sent.Channel != "discord" || sent.ChatID != "222" {
		t.Errorf("sent = %+v, expected discord:222 (latest context)", sent)
	}
}
