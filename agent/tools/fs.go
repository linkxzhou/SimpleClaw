package tools

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// ============ ReadFileTool ============

// ReadFileTool 读取文件内容
type ReadFileTool struct{}

func (t *ReadFileTool) Name() string        { return "read_file" }
func (t *ReadFileTool) Description() string { return "Read the contents of a file at the given path." }
func (t *ReadFileTool) Parameters() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"path": map[string]interface{}{
				"type":        "string",
				"description": "The file path to read",
			},
		},
		"required": []string{"path"},
	}
}

func (t *ReadFileTool) Execute(_ context.Context, params map[string]interface{}) (string, error) {
	path, _ := params["path"].(string)
	if path == "" {
		return "", fmt.Errorf("path is required")
	}

	filePath, err := filepath.Abs(path)
	if err != nil {
		return "", fmt.Errorf("invalid path: %w", err)
	}

	info, err := os.Stat(filePath)
	if os.IsNotExist(err) {
		return fmt.Sprintf("Error: File not found: %s", path), nil
	}
	if err != nil {
		return "", err
	}
	if info.IsDir() {
		return fmt.Sprintf("Error: Not a file: %s", path), nil
	}

	content, err := os.ReadFile(filePath)
	if err != nil {
		if os.IsPermission(err) {
			return fmt.Sprintf("Error: Permission denied: %s", path), nil
		}
		return "", fmt.Errorf("failed to read file: %w", err)
	}
	return string(content), nil
}

// ============ WriteFileTool ============

// WriteFileTool 写入文件内容
type WriteFileTool struct{}

func (t *WriteFileTool) Name() string        { return "write_file" }
func (t *WriteFileTool) Description() string {
	return "Write content to a file at the given path. Creates parent directories if needed."
}
func (t *WriteFileTool) Parameters() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"path": map[string]interface{}{
				"type":        "string",
				"description": "The file path to write to",
			},
			"content": map[string]interface{}{
				"type":        "string",
				"description": "The content to write",
			},
		},
		"required": []string{"path", "content"},
	}
}

func (t *WriteFileTool) Execute(_ context.Context, params map[string]interface{}) (string, error) {
	path, _ := params["path"].(string)
	content, _ := params["content"].(string)
	if path == "" {
		return "", fmt.Errorf("path is required")
	}

	filePath, err := filepath.Abs(path)
	if err != nil {
		return "", fmt.Errorf("invalid path: %w", err)
	}

	if err := os.MkdirAll(filepath.Dir(filePath), 0755); err != nil {
		return "", fmt.Errorf("failed to create directory: %w", err)
	}

	if err := os.WriteFile(filePath, []byte(content), 0644); err != nil {
		if os.IsPermission(err) {
			return fmt.Sprintf("Error: Permission denied: %s", path), nil
		}
		return "", fmt.Errorf("failed to write file: %w", err)
	}
	return fmt.Sprintf("Successfully wrote %d bytes to %s", len(content), path), nil
}

// ============ EditFileTool ============

// EditFileTool 通过文本替换编辑文件
type EditFileTool struct{}

func (t *EditFileTool) Name() string        { return "edit_file" }
func (t *EditFileTool) Description() string {
	return "Edit a file by replacing old_text with new_text. The old_text must exist exactly in the file."
}
func (t *EditFileTool) Parameters() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"path": map[string]interface{}{
				"type":        "string",
				"description": "The file path to edit",
			},
			"old_text": map[string]interface{}{
				"type":        "string",
				"description": "The exact text to find and replace",
			},
			"new_text": map[string]interface{}{
				"type":        "string",
				"description": "The text to replace with",
			},
		},
		"required": []string{"path", "old_text", "new_text"},
	}
}

func (t *EditFileTool) Execute(_ context.Context, params map[string]interface{}) (string, error) {
	path, _ := params["path"].(string)
	oldText, _ := params["old_text"].(string)
	newText, _ := params["new_text"].(string)
	if path == "" || oldText == "" {
		return "", fmt.Errorf("path and old_text are required")
	}

	filePath, err := filepath.Abs(path)
	if err != nil {
		return "", fmt.Errorf("invalid path: %w", err)
	}

	content, err := os.ReadFile(filePath)
	if os.IsNotExist(err) {
		return fmt.Sprintf("Error: File not found: %s", path), nil
	}
	if err != nil {
		return "", err
	}

	fileContent := string(content)
	if !strings.Contains(fileContent, oldText) {
		return "Error: old_text not found in file. Make sure it matches exactly.", nil
	}

	count := strings.Count(fileContent, oldText)
	if count > 1 {
		return fmt.Sprintf("Warning: old_text appears %d times. Please provide more context to make it unique.", count), nil
	}

	newContent := strings.Replace(fileContent, oldText, newText, 1)
	if err := os.WriteFile(filePath, []byte(newContent), 0644); err != nil {
		return "", fmt.Errorf("failed to write file: %w", err)
	}
	return fmt.Sprintf("Successfully edited %s", path), nil
}

// ============ ListDirTool ============

// ListDirTool 列出目录内容
type ListDirTool struct{}

func (t *ListDirTool) Name() string        { return "list_dir" }
func (t *ListDirTool) Description() string { return "List the contents of a directory." }
func (t *ListDirTool) Parameters() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"path": map[string]interface{}{
				"type":        "string",
				"description": "The directory path to list",
			},
		},
		"required": []string{"path"},
	}
}

func (t *ListDirTool) Execute(_ context.Context, params map[string]interface{}) (string, error) {
	path, _ := params["path"].(string)
	if path == "" {
		return "", fmt.Errorf("path is required")
	}

	dirPath, err := filepath.Abs(path)
	if err != nil {
		return "", fmt.Errorf("invalid path: %w", err)
	}

	info, err := os.Stat(dirPath)
	if os.IsNotExist(err) {
		return fmt.Sprintf("Error: Directory not found: %s", path), nil
	}
	if err != nil {
		return "", err
	}
	if !info.IsDir() {
		return fmt.Sprintf("Error: Not a directory: %s", path), nil
	}

	entries, err := os.ReadDir(dirPath)
	if err != nil {
		if os.IsPermission(err) {
			return fmt.Sprintf("Error: Permission denied: %s", path), nil
		}
		return "", err
	}

	if len(entries) == 0 {
		return fmt.Sprintf("Directory %s is empty", path), nil
	}

	var items []string
	for _, entry := range entries {
		prefix := "[file] "
		if entry.IsDir() {
			prefix = "[dir]  "
		}
		items = append(items, prefix+entry.Name())
	}
	return strings.Join(items, "\n"), nil
}
