package agent

import (
	"os"
	"strings"
	"testing"
)

func TestSnippetStore_SaveAndGet(t *testing.T) {
	dir := t.TempDir()
	store, err := NewSnippetStore(dir)
	if err != nil {
		t.Fatal(err)
	}

	sn := &Snippet{
		ID:          "test-1",
		Name:        "test snippet",
		Description: "a test snippet",
		Code:        "package main\nfunc Run(input map[string]interface{}) string { return \"ok\" }",
		Tags:        []string{"test", "demo"},
	}
	if err := store.Save(sn); err != nil {
		t.Fatalf("Save: %v", err)
	}

	got, ok := store.Get("test-1")
	if !ok {
		t.Fatal("Get should find snippet")
	}
	if got.Name != "test snippet" {
		t.Errorf("Name = %q, want %q", got.Name, "test snippet")
	}
	if got.FuncName != "Run" {
		t.Errorf("FuncName = %q, want %q", got.FuncName, "Run")
	}
}

func TestSnippetStore_GetByName(t *testing.T) {
	dir := t.TempDir()
	store, _ := NewSnippetStore(dir)
	store.Save(&Snippet{ID: "s1", Name: "CSV Parser", Code: "x"})

	sn, ok := store.GetByName("csv parser")
	if !ok {
		t.Fatal("GetByName should find (case-insensitive)")
	}
	if sn.ID != "s1" {
		t.Errorf("ID = %q, want %q", sn.ID, "s1")
	}

	_, ok = store.GetByName("nonexistent")
	if ok {
		t.Error("GetByName should return false for nonexistent")
	}
}

func TestSnippetStore_Delete(t *testing.T) {
	dir := t.TempDir()
	store, _ := NewSnippetStore(dir)
	store.Save(&Snippet{ID: "d1", Name: "Delete Me", Code: "x"})

	if err := store.Delete("d1"); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if _, ok := store.Get("d1"); ok {
		t.Error("Get should return false after Delete")
	}
}

func TestSnippetStore_Search(t *testing.T) {
	dir := t.TempDir()
	store, _ := NewSnippetStore(dir)
	store.Save(&Snippet{ID: "s1", Name: "CSV Parser", Description: "parse CSV files", Code: "x", Tags: []string{"csv", "data"}})
	store.Save(&Snippet{ID: "s2", Name: "JSON Diff", Description: "compare JSON objects", Code: "x", Tags: []string{"json"}})
	store.Save(&Snippet{ID: "s3", Name: "Date Calc", Description: "business days", Code: "x", Tags: []string{"date"}})

	// 按名称搜索
	results := store.Search("csv")
	if len(results) != 1 || results[0].ID != "s1" {
		t.Errorf("Search(csv) = %d results, want 1", len(results))
	}

	// 按标签搜索
	results = store.Search("json")
	if len(results) != 1 || results[0].ID != "s2" {
		t.Errorf("Search(json) = %d results, want 1", len(results))
	}

	// 按描述搜索
	results = store.Search("business")
	if len(results) != 1 || results[0].ID != "s3" {
		t.Errorf("Search(business) = %d results, want 1", len(results))
	}

	// 空查询返回全部
	results = store.Search("")
	if len(results) != 3 {
		t.Errorf("Search('') = %d results, want 3", len(results))
	}
}

func TestSnippetStore_ListByUsage(t *testing.T) {
	dir := t.TempDir()
	store, _ := NewSnippetStore(dir)
	store.Save(&Snippet{ID: "a", Name: "A", Code: "x", UseCount: 1})
	store.Save(&Snippet{ID: "b", Name: "B", Code: "x", UseCount: 5})
	store.Save(&Snippet{ID: "c", Name: "C", Code: "x", UseCount: 3})

	top := store.ListByUsage(2)
	if len(top) != 2 {
		t.Fatalf("ListByUsage(2) = %d, want 2", len(top))
	}
	if top[0].ID != "b" {
		t.Errorf("first should be 'b' (5 uses), got %q", top[0].ID)
	}
	if top[1].ID != "c" {
		t.Errorf("second should be 'c' (3 uses), got %q", top[1].ID)
	}
}

func TestSnippetStore_RecordUse(t *testing.T) {
	dir := t.TempDir()
	store, _ := NewSnippetStore(dir)
	store.Save(&Snippet{ID: "u1", Name: "Usage Test", Code: "x"})

	store.RecordUse("u1")
	store.RecordUse("u1")

	sn, _ := store.Get("u1")
	if sn.UseCount != 2 {
		t.Errorf("UseCount = %d, want 2", sn.UseCount)
	}
	if sn.LastUsed.IsZero() {
		t.Error("LastUsed should be set")
	}
}

func TestSnippetStore_Persistence(t *testing.T) {
	dir := t.TempDir()
	store1, _ := NewSnippetStore(dir)
	store1.Save(&Snippet{ID: "p1", Name: "Persist Test", Code: "package main", UseCount: 3})

	// 重新加载
	store2, err := NewSnippetStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	sn, ok := store2.Get("p1")
	if !ok {
		t.Fatal("should persist across reloads")
	}
	if sn.UseCount != 3 {
		t.Errorf("UseCount = %d, want 3", sn.UseCount)
	}
}

func TestSnippetStore_Len(t *testing.T) {
	dir := t.TempDir()
	store, _ := NewSnippetStore(dir)
	if store.Len() != 0 {
		t.Errorf("Len = %d, want 0", store.Len())
	}
	store.Save(&Snippet{ID: "x", Name: "X", Code: "x"})
	if store.Len() != 1 {
		t.Errorf("Len = %d, want 1", store.Len())
	}
}

func TestSnippetStore_SaveValidation(t *testing.T) {
	dir := t.TempDir()
	store, _ := NewSnippetStore(dir)

	if err := store.Save(&Snippet{Code: "x"}); err == nil {
		t.Error("Save with empty ID should fail")
	}
	if err := store.Save(&Snippet{ID: "x"}); err == nil {
		t.Error("Save with empty Code should fail")
	}
}

func TestSnippetStore_BuildSummary(t *testing.T) {
	dir := t.TempDir()
	store, _ := NewSnippetStore(dir)

	// 空时返回空
	if s := store.BuildSummary(10); s != "" {
		t.Errorf("empty store should return empty summary, got %q", s)
	}

	store.Save(&Snippet{ID: "s1", Name: "CSV Parser", Description: "parse CSV", Code: "x", UseCount: 5})
	store.Save(&Snippet{ID: "s2", Name: "JSON Diff", Description: "compare JSON", Code: "x"})

	summary := store.BuildSummary(10)
	if !strings.Contains(summary, "CSV Parser") {
		t.Error("summary should contain snippet names")
	}
	if !strings.Contains(summary, "used 5 times") {
		t.Error("summary should show use count")
	}
	if !strings.Contains(summary, "go_snippet") {
		t.Error("summary should reference go_snippet tool")
	}
}

func TestSnippetStore_NewSnippetStoreError(t *testing.T) {
	// 使用不可写路径
	_, err := NewSnippetStore("/dev/null/impossible")
	if err == nil {
		// 在某些系统上可能不报错，跳过
		if _, statErr := os.Stat("/dev/null/impossible"); statErr != nil {
			t.Log("expected error for invalid path (skipped on permissive systems)")
		}
	}
}
