package repository

import (
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/marqsleal/api-2-tool/internal/domain"
	_ "modernc.org/sqlite"
)

func TestInMemoryRepositoryLifecycle(t *testing.T) {
	repo := NewInMemoryToolDefinitionRepository()

	created, err := repo.Create(domain.ToolDefinition{Name: "n", Method: "GET", URL: "https://x", Active: false})
	if err != nil {
		t.Fatalf("create error: %v", err)
	}
	if !created.Active || created.ID == "" {
		t.Fatalf("expected active created tool with id")
	}

	list, err := repo.List()
	if err != nil || len(list) != 1 {
		t.Fatalf("expected one active item")
	}

	got, ok, err := repo.GetByID(created.ID)
	if err != nil || !ok || got.ID != created.ID {
		t.Fatalf("expected get by id")
	}

	name := "new"
	strict := true
	patched, ok, err := repo.Patch(created.ID, domain.ToolDefinitionPatch{Name: &name, Strict: &strict})
	if err != nil || !ok || patched.Name != "new" || !patched.Strict {
		t.Fatalf("expected patch success")
	}

	ok, err = repo.Deactivate(created.ID)
	if err != nil || !ok {
		t.Fatalf("expected deactivate success")
	}

	list, _ = repo.List()
	if len(list) != 0 {
		t.Fatalf("expected filtered inactive items")
	}

	_, ok, _ = repo.GetByID(created.ID)
	if ok {
		t.Fatalf("expected inactive tool not found")
	}

	_, ok, _ = repo.Patch(created.ID, domain.ToolDefinitionPatch{Name: &name})
	if ok {
		t.Fatalf("expected patch false for inactive")
	}

	ok, _ = repo.Deactivate("missing")
	if ok {
		t.Fatalf("expected deactivate false for missing")
	}
}

func TestSQLiteRepositoryLifecycleAndErrors(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "tools.db")
	repo, err := NewSQLiteToolDefinitionRepository(dbPath)
	if err != nil {
		t.Fatalf("new sqlite repo error: %v", err)
	}
	defer repo.Close()

	created, err := repo.Create(domain.ToolDefinition{
		Name:        "n",
		Description: "d",
		Method:      "GET",
		URL:         "https://x",
		Headers:     map[string]string{"a": "b"},
		Parameters:  map[string]any{"k": "v"},
		Strict:      false,
	})
	if err != nil {
		t.Fatalf("create error: %v", err)
	}
	if !created.Active || created.ID == "" {
		t.Fatalf("expected active and id")
	}

	list, err := repo.List()
	if err != nil || len(list) != 1 {
		t.Fatalf("expected list with one item")
	}

	got, ok, err := repo.GetByID(created.ID)
	if err != nil || !ok || got.ID != created.ID {
		t.Fatalf("expected get by id")
	}

	_, ok, err = repo.GetByID("missing")
	if err != nil || ok {
		t.Fatalf("expected missing false")
	}

	method := "POST"
	strict := true
	params := map[string]any{"changed": true}
	patched, ok, err := repo.Patch(created.ID, domain.ToolDefinitionPatch{Method: &method, Strict: &strict, Parameters: &params})
	if err != nil || !ok || patched.Method != "POST" || !patched.Strict {
		t.Fatalf("expected patch success")
	}

	_, ok, err = repo.Patch("missing", domain.ToolDefinitionPatch{Method: &method})
	if err != nil || ok {
		t.Fatalf("expected patch false for missing")
	}

	_, ok, err = repo.Patch(created.ID, domain.ToolDefinitionPatch{})
	if err != nil || ok {
		t.Fatalf("expected patch false when no fields")
	}

	name := "new_name"
	description := "new_description"
	url := "https://new-url"
	headers := map[string]string{"h": "v"}
	_, ok, err = repo.Patch(created.ID, domain.ToolDefinitionPatch{Name: &name, Description: &description, URL: &url, Headers: &headers})
	if err != nil || !ok {
		t.Fatalf("expected patch success for name/description/url/headers")
	}

	ok, err = repo.Deactivate(created.ID)
	if err != nil || !ok {
		t.Fatalf("expected deactivate success")
	}

	ok, err = repo.Deactivate(created.ID)
	if err != nil || ok {
		t.Fatalf("expected deactivate false for inactive")
	}

	list, _ = repo.List()
	if len(list) != 0 {
		t.Fatalf("expected inactive filtered from list")
	}

	_, ok, _ = repo.GetByID(created.ID)
	if ok {
		t.Fatalf("expected inactive hidden on get")
	}

	if _, err := repo.Create(domain.ToolDefinition{Name: "bad", Method: "POST", URL: "https://x", Parameters: map[string]any{"bad": make(chan int)}}); err == nil {
		t.Fatalf("expected create marshal error")
	}

	badParams := map[string]any{"bad": make(chan int)}
	_, _, err = repo.Patch("missing", domain.ToolDefinitionPatch{Parameters: &badParams})
	if err == nil {
		t.Fatalf("expected patch marshal error")
	}
}

func TestSQLiteRepositoryMigrateOldSchemaAndHelpers(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "old.db")
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	defer db.Close()

	_, err = db.Exec(`
CREATE TABLE tool_definitions (
	internal_id INTEGER PRIMARY KEY AUTOINCREMENT,
	id TEXT NOT NULL UNIQUE,
	name TEXT NOT NULL,
	description TEXT NOT NULL DEFAULT '',
	method TEXT NOT NULL,
	url TEXT NOT NULL,
	headers_json TEXT NOT NULL,
	parameters_json TEXT NOT NULL,
	strict INTEGER NOT NULL DEFAULT 0
);`)
	if err != nil {
		t.Fatalf("create old table: %v", err)
	}

	repo := &SQLiteToolDefinitionRepository{db: db}
	if err := repo.migrate(); err != nil {
		t.Fatalf("migrate error: %v", err)
	}

	hasActive, err := repo.hasColumn("tool_definitions", "active")
	if err != nil || !hasActive {
		t.Fatalf("expected active column after migration")
	}

	hasMissing, err := repo.hasColumn("tool_definitions", "missing_col")
	if err != nil {
		t.Fatalf("hasColumn missing err: %v", err)
	}
	if hasMissing {
		t.Fatalf("did not expect missing column")
	}

	if err := ensureDir(filepath.Join(t.TempDir(), "a", "b", "c.db")); err != nil {
		t.Fatalf("ensureDir error: %v", err)
	}
	if err := ensureDir("local.db"); err != nil {
		t.Fatalf("ensureDir local error: %v", err)
	}

	if got := boolToInt(true); got != 1 {
		t.Fatalf("expected 1")
	}
	if got := boolToInt(false); got != 0 {
		t.Fatalf("expected 0")
	}

	_ = repo.Close()
	if _, err := repo.List(); err == nil {
		t.Fatalf("expected list error on closed db")
	}

}

type fakeScanner struct {
	values []any
	err    error
}

func (f fakeScanner) Scan(dest ...any) error {
	if f.err != nil {
		return f.err
	}
	if len(dest) != len(f.values) {
		return fmt.Errorf("scan length mismatch")
	}
	for i := range dest {
		switch d := dest[i].(type) {
		case *string:
			*d = f.values[i].(string)
		case *int:
			*d = f.values[i].(int)
		default:
			return fmt.Errorf("unsupported type")
		}
	}
	return nil
}

func TestScanDefinition(t *testing.T) {
	okScanner := fakeScanner{values: []any{"id", "name", "desc", "GET", "https://x", `{"k":"v"}`, `{"p":1}`, 1, 1}}
	def, err := scanDefinition(okScanner)
	if err != nil {
		t.Fatalf("unexpected scanDefinition error: %v", err)
	}
	if def.ID != "id" || !def.Strict || !def.Active {
		t.Fatalf("unexpected definition: %+v", def)
	}

	if _, err := scanDefinition(fakeScanner{err: errors.New("scan")}); err == nil {
		t.Fatalf("expected scan error")
	}

	if _, err := scanDefinition(fakeScanner{values: []any{"id", "name", "desc", "GET", "https://x", `bad`, `{"p":1}`, 1, 1}}); err == nil {
		t.Fatalf("expected headers unmarshal error")
	}

	if _, err := scanDefinition(fakeScanner{values: []any{"id", "name", "desc", "GET", "https://x", `{"k":"v"}`, `bad`, 1, 1}}); err == nil {
		t.Fatalf("expected parameters unmarshal error")
	}
}

func TestMkdirAllHelper(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "x", "y")
	if err := mkdirAll(dir); err != nil {
		t.Fatalf("mkdirAll error: %v", err)
	}
	if _, err := os.Stat(dir); err != nil {
		t.Fatalf("expected dir to exist: %v", err)
	}
}

func TestSQLiteRepositoryConstructorAndFailurePaths(t *testing.T) {
	badDirPath := filepath.Join("/proc", "1", "forbidden", "tools.db")
	if _, err := NewSQLiteToolDefinitionRepository(badDirPath); err == nil {
		t.Fatalf("expected constructor error for forbidden dir")
	}

	dirAsDB := t.TempDir()
	if _, err := NewSQLiteToolDefinitionRepository(dirAsDB); err == nil {
		t.Fatalf("expected constructor error when db path is a directory")
	}

	dbPath := filepath.Join(t.TempDir(), "x.db")
	repo, err := NewSQLiteToolDefinitionRepository(dbPath)
	if err != nil {
		t.Fatalf("new repo error: %v", err)
	}

	_ = repo.Close()
	if _, err := repo.Create(domain.ToolDefinition{Name: "x", Method: "GET", URL: "https://x", Parameters: map[string]any{}}); err == nil {
		t.Fatalf("expected create error on closed db")
	}
	if _, _, err := repo.GetByID("tool_1"); err == nil {
		t.Fatalf("expected get error on closed db")
	}
	if _, _, err := repo.Patch("tool_1", domain.ToolDefinitionPatch{Method: ptrString("GET")}); err == nil {
		t.Fatalf("expected patch db error on closed db")
	}
	if _, err := repo.Deactivate("tool_1"); err == nil {
		t.Fatalf("expected deactivate db error on closed db")
	}

	if _, err := repo.hasColumn("tool_definitions", "active"); err == nil {
		t.Fatalf("expected hasColumn error on closed db")
	}
}

func TestInMemoryPatchAllFields(t *testing.T) {
	repo := NewInMemoryToolDefinitionRepository()
	created, _ := repo.Create(domain.ToolDefinition{Name: "a", Description: "d", Method: "GET", URL: "https://x", Headers: map[string]string{}, Parameters: map[string]any{}, Strict: false})

	name := "b"
	desc := "d2"
	method := "POST"
	url := "https://y"
	headers := map[string]string{"k": "v"}
	params := map[string]any{"p": true}
	strict := true

	patched, ok, err := repo.Patch(created.ID, domain.ToolDefinitionPatch{
		Name:        &name,
		Description: &desc,
		Method:      &method,
		URL:         &url,
		Headers:     &headers,
		Parameters:  &params,
		Strict:      &strict,
	})
	if err != nil || !ok {
		t.Fatalf("expected patch success")
	}
	if patched.Name != name || patched.Description != desc || patched.Method != method || patched.URL != url || !patched.Strict {
		t.Fatalf("unexpected patched values: %+v", patched)
	}
}

func ptrString(v string) *string { return &v }

func TestSQLiteRepositoryListWithCorruptRow(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "corrupt.db")
	repo, err := NewSQLiteToolDefinitionRepository(dbPath)
	if err != nil {
		t.Fatalf("new repo error: %v", err)
	}
	defer repo.Close()

	_, err = repo.db.Exec(
		`INSERT INTO tool_definitions (id, name, description, method, url, headers_json, parameters_json, strict, active)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		"tool_corrupt",
		"n",
		"d",
		"GET",
		"https://x",
		"{bad-json",
		`{"x":1}`,
		0,
		1,
	)
	if err != nil {
		t.Fatalf("insert corrupt row error: %v", err)
	}

	if _, err := repo.List(); err == nil {
		t.Fatalf("expected list error for corrupt row")
	}
}
