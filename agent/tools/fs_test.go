package tools

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// ---------- ReadFileTool ----------

func TestReadFileTool_Name(t *testing.T) {
	tool := &ReadFileTool{}
	if tool.Name() != "read_file" {
		t.Errorf("Name = %q, want %q", tool.Name(), "read_file")
	}
}

func TestReadFileTool_Success(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.txt")
	os.WriteFile(path, []byte("hello world"), 0644)

	tool := &ReadFileTool{}
	result, err := tool.Execute(context.Background(), map[string]interface{}{"path": path})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != "hello world" {
		t.Errorf("result = %q, want %q", result, "hello world")
	}
}

func TestReadFileTool_EmptyPath(t *testing.T) {
	tool := &ReadFileTool{}
	_, err := tool.Execute(context.Background(), map[string]interface{}{"path": ""})
	if err == nil {
		t.Fatal("expected error for empty path")
	}
}

func TestReadFileTool_FileNotFound(t *testing.T) {
	tool := &ReadFileTool{}
	result, err := tool.Execute(context.Background(), map[string]interface{}{"path": "/nonexistent/file.txt"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(result, "Error: File not found") {
		t.Errorf("result = %q, expected file not found error", result)
	}
}

func TestReadFileTool_ReadDirectory(t *testing.T) {
	dir := t.TempDir()
	tool := &ReadFileTool{}
	result, err := tool.Execute(context.Background(), map[string]interface{}{"path": dir})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(result, "Error: Not a file") {
		t.Errorf("result = %q, expected not-a-file error", result)
	}
}

func TestReadFileTool_Parameters(t *testing.T) {
	tool := &ReadFileTool{}
	params := tool.Parameters()
	if params["type"] != "object" {
		t.Errorf("Parameters type = %v, want object", params["type"])
	}
}

// ---------- WriteFileTool ----------

func TestWriteFileTool_Name(t *testing.T) {
	tool := &WriteFileTool{}
	if tool.Name() != "write_file" {
		t.Errorf("Name = %q, want %q", tool.Name(), "write_file")
	}
}

func TestWriteFileTool_Success(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "output.txt")

	tool := &WriteFileTool{}
	result, err := tool.Execute(context.Background(), map[string]interface{}{
		"path":    path,
		"content": "hello world",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(result, "Successfully wrote") {
		t.Errorf("result = %q, expected success message", result)
	}

	content, _ := os.ReadFile(path)
	if string(content) != "hello world" {
		t.Errorf("file content = %q, want %q", string(content), "hello world")
	}
}

func TestWriteFileTool_CreateParentDirs(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "sub", "dir", "file.txt")

	tool := &WriteFileTool{}
	_, err := tool.Execute(context.Background(), map[string]interface{}{
		"path":    path,
		"content": "nested",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	content, _ := os.ReadFile(path)
	if string(content) != "nested" {
		t.Errorf("file content = %q, want %q", string(content), "nested")
	}
}

func TestWriteFileTool_EmptyPath(t *testing.T) {
	tool := &WriteFileTool{}
	_, err := tool.Execute(context.Background(), map[string]interface{}{
		"path":    "",
		"content": "data",
	})
	if err == nil {
		t.Fatal("expected error for empty path")
	}
}

func TestWriteFileTool_EmptyContent(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "empty.txt")

	tool := &WriteFileTool{}
	result, err := tool.Execute(context.Background(), map[string]interface{}{
		"path":    path,
		"content": "",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(result, "Successfully wrote 0 bytes") {
		t.Errorf("result = %q, expected 0 bytes written", result)
	}
}

// ---------- EditFileTool ----------

func TestEditFileTool_Name(t *testing.T) {
	tool := &EditFileTool{}
	if tool.Name() != "edit_file" {
		t.Errorf("Name = %q, want %q", tool.Name(), "edit_file")
	}
}

func TestEditFileTool_Success(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "edit.txt")
	os.WriteFile(path, []byte("hello world"), 0644)

	tool := &EditFileTool{}
	result, err := tool.Execute(context.Background(), map[string]interface{}{
		"path":     path,
		"old_text": "world",
		"new_text": "Go",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(result, "Successfully edited") {
		t.Errorf("result = %q, expected success message", result)
	}

	content, _ := os.ReadFile(path)
	if string(content) != "hello Go" {
		t.Errorf("file content = %q, want %q", string(content), "hello Go")
	}
}

func TestEditFileTool_OldTextNotFound(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "edit.txt")
	os.WriteFile(path, []byte("hello world"), 0644)

	tool := &EditFileTool{}
	result, err := tool.Execute(context.Background(), map[string]interface{}{
		"path":     path,
		"old_text": "nonexistent",
		"new_text": "replaced",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(result, "old_text not found") {
		t.Errorf("result = %q, expected not-found error", result)
	}
}

func TestEditFileTool_MultipleOccurrences(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "edit.txt")
	os.WriteFile(path, []byte("foo bar foo"), 0644)

	tool := &EditFileTool{}
	result, err := tool.Execute(context.Background(), map[string]interface{}{
		"path":     path,
		"old_text": "foo",
		"new_text": "baz",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(result, "appears 2 times") {
		t.Errorf("result = %q, expected multiple-occurrences warning", result)
	}

	// 文件内容不应被修改
	content, _ := os.ReadFile(path)
	if string(content) != "foo bar foo" {
		t.Errorf("file should not be modified, got %q", string(content))
	}
}

func TestEditFileTool_FileNotFound(t *testing.T) {
	tool := &EditFileTool{}
	result, err := tool.Execute(context.Background(), map[string]interface{}{
		"path":     "/nonexistent/file.txt",
		"old_text": "foo",
		"new_text": "bar",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(result, "File not found") {
		t.Errorf("result = %q, expected file-not-found error", result)
	}
}

func TestEditFileTool_EmptyPath(t *testing.T) {
	tool := &EditFileTool{}
	_, err := tool.Execute(context.Background(), map[string]interface{}{
		"path":     "",
		"old_text": "foo",
		"new_text": "bar",
	})
	if err == nil {
		t.Fatal("expected error for empty path")
	}
}

func TestEditFileTool_EmptyOldText(t *testing.T) {
	tool := &EditFileTool{}
	_, err := tool.Execute(context.Background(), map[string]interface{}{
		"path":     "/tmp/test.txt",
		"old_text": "",
		"new_text": "bar",
	})
	if err == nil {
		t.Fatal("expected error for empty old_text")
	}
}

// ---------- ListDirTool ----------

func TestListDirTool_Name(t *testing.T) {
	tool := &ListDirTool{}
	if tool.Name() != "list_dir" {
		t.Errorf("Name = %q, want %q", tool.Name(), "list_dir")
	}
}

func TestListDirTool_Success(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "file1.txt"), []byte("a"), 0644)
	os.Mkdir(filepath.Join(dir, "subdir"), 0755)

	tool := &ListDirTool{}
	result, err := tool.Execute(context.Background(), map[string]interface{}{"path": dir})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(result, "file1.txt") {
		t.Errorf("result should contain file1.txt, got %q", result)
	}
	if !strings.Contains(result, "subdir") {
		t.Errorf("result should contain subdir, got %q", result)
	}
	if !strings.Contains(result, "[file]") {
		t.Errorf("result should contain [file] prefix, got %q", result)
	}
	if !strings.Contains(result, "[dir]") {
		t.Errorf("result should contain [dir] prefix, got %q", result)
	}
}

func TestListDirTool_EmptyDir(t *testing.T) {
	dir := t.TempDir()
	tool := &ListDirTool{}
	result, err := tool.Execute(context.Background(), map[string]interface{}{"path": dir})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(result, "is empty") {
		t.Errorf("result = %q, expected empty directory message", result)
	}
}

func TestListDirTool_NotFound(t *testing.T) {
	tool := &ListDirTool{}
	result, err := tool.Execute(context.Background(), map[string]interface{}{"path": "/nonexistent/dir"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(result, "Directory not found") {
		t.Errorf("result = %q, expected not-found error", result)
	}
}

func TestListDirTool_NotADirectory(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "file.txt")
	os.WriteFile(path, []byte("a"), 0644)

	tool := &ListDirTool{}
	result, err := tool.Execute(context.Background(), map[string]interface{}{"path": path})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(result, "Not a directory") {
		t.Errorf("result = %q, expected not-a-directory error", result)
	}
}

func TestListDirTool_EmptyPath(t *testing.T) {
	tool := &ListDirTool{}
	_, err := tool.Execute(context.Background(), map[string]interface{}{"path": ""})
	if err == nil {
		t.Fatal("expected error for empty path")
	}
}
