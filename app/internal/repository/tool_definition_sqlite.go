package repository

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/google/uuid"

	_ "modernc.org/sqlite"

	"github.com/marqsleal/api-2-tool/internal/domain"
)

type SQLiteToolDefinitionRepository struct {
	db *sql.DB
}

func NewSQLiteToolDefinitionRepository(dbPath string) (*SQLiteToolDefinitionRepository, error) {
	if err := ensureDir(dbPath); err != nil {
		return nil, err
	}

	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}

	repo := &SQLiteToolDefinitionRepository{db: db}
	if err := repo.migrate(); err != nil {
		_ = db.Close()
		return nil, err
	}

	return repo, nil
}

func (r *SQLiteToolDefinitionRepository) Close() error {
	return r.db.Close()
}

func (r *SQLiteToolDefinitionRepository) Create(definition domain.ToolDefinition) (domain.ToolDefinition, error) {
	headersJSON, err := json.Marshal(definition.Headers)
	if err != nil {
		return domain.ToolDefinition{}, fmt.Errorf("marshal headers: %w", err)
	}

	parametersJSON, err := json.Marshal(definition.Parameters)
	if err != nil {
		return domain.ToolDefinition{}, fmt.Errorf("marshal parameters: %w", err)
	}

	definition.ID = "tool_" + uuid.NewString()
	definition.Active = true
	_, err = r.db.Exec(
		`INSERT INTO tool_definitions (id, name, description, method, url, headers_json, parameters_json, strict, active)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		definition.ID,
		definition.Name,
		definition.Description,
		definition.Method,
		definition.URL,
		string(headersJSON),
		string(parametersJSON),
		boolToInt(definition.Strict),
		boolToInt(definition.Active),
	)
	if err != nil {
		return domain.ToolDefinition{}, fmt.Errorf("insert definition: %w", err)
	}

	return definition, nil
}

func (r *SQLiteToolDefinitionRepository) List() ([]domain.ToolDefinition, error) {
	rows, err := r.db.Query(
		`SELECT id, name, description, method, url, headers_json, parameters_json, strict, active
		 FROM tool_definitions
		 WHERE active = 1
		 ORDER BY internal_id ASC`,
	)
	if err != nil {
		return nil, fmt.Errorf("query definitions: %w", err)
	}
	defer rows.Close()

	definitions := make([]domain.ToolDefinition, 0)
	for rows.Next() {
		definition, err := scanDefinition(rows)
		if err != nil {
			return nil, err
		}
		definitions = append(definitions, definition)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate definitions: %w", err)
	}

	return definitions, nil
}

func (r *SQLiteToolDefinitionRepository) GetByID(id string) (domain.ToolDefinition, bool, error) {
	row := r.db.QueryRow(
		`SELECT id, name, description, method, url, headers_json, parameters_json, strict, active
		 FROM tool_definitions
		 WHERE id = ? AND active = 1`,
		id,
	)

	definition, err := scanDefinition(row)
	if err != nil {
		if err == sql.ErrNoRows {
			return domain.ToolDefinition{}, false, nil
		}
		return domain.ToolDefinition{}, false, err
	}

	return definition, true, nil
}

func (r *SQLiteToolDefinitionRepository) Patch(id string, patch domain.ToolDefinitionPatch) (domain.ToolDefinition, bool, error) {
	setClauses := make([]string, 0, 7)
	args := make([]any, 0, 8)

	if patch.Name != nil {
		setClauses = append(setClauses, "name = ?")
		args = append(args, *patch.Name)
	}
	if patch.Description != nil {
		setClauses = append(setClauses, "description = ?")
		args = append(args, *patch.Description)
	}
	if patch.Method != nil {
		setClauses = append(setClauses, "method = ?")
		args = append(args, *patch.Method)
	}
	if patch.URL != nil {
		setClauses = append(setClauses, "url = ?")
		args = append(args, *patch.URL)
	}
	if patch.Headers != nil {
		headersJSON, err := json.Marshal(*patch.Headers)
		if err != nil {
			return domain.ToolDefinition{}, false, fmt.Errorf("marshal headers: %w", err)
		}
		setClauses = append(setClauses, "headers_json = ?")
		args = append(args, string(headersJSON))
	}
	if patch.Parameters != nil {
		parametersJSON, err := json.Marshal(*patch.Parameters)
		if err != nil {
			return domain.ToolDefinition{}, false, fmt.Errorf("marshal parameters: %w", err)
		}
		setClauses = append(setClauses, "parameters_json = ?")
		args = append(args, string(parametersJSON))
	}
	if patch.Strict != nil {
		setClauses = append(setClauses, "strict = ?")
		args = append(args, boolToInt(*patch.Strict))
	}

	if len(setClauses) == 0 {
		return domain.ToolDefinition{}, false, nil
	}

	query := fmt.Sprintf(
		"UPDATE tool_definitions SET %s WHERE id = ? AND active = 1",
		strings.Join(setClauses, ", "),
	)
	args = append(args, id)

	result, err := r.db.Exec(query, args...)
	if err != nil {
		return domain.ToolDefinition{}, false, fmt.Errorf("update definition: %w", err)
	}

	affected, err := result.RowsAffected()
	if err != nil {
		return domain.ToolDefinition{}, false, fmt.Errorf("rows affected: %w", err)
	}
	if affected == 0 {
		return domain.ToolDefinition{}, false, nil
	}

	definition, ok, err := r.GetByID(id)
	if err != nil {
		return domain.ToolDefinition{}, false, err
	}
	if !ok {
		return domain.ToolDefinition{}, false, nil
	}

	return definition, true, nil
}

func (r *SQLiteToolDefinitionRepository) Deactivate(id string) (bool, error) {
	result, err := r.db.Exec(
		"UPDATE tool_definitions SET active = 0 WHERE id = ? AND active = 1",
		id,
	)
	if err != nil {
		return false, fmt.Errorf("deactivate definition: %w", err)
	}

	affected, err := result.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("rows affected: %w", err)
	}

	return affected > 0, nil
}

type scanner interface {
	Scan(dest ...any) error
}

func scanDefinition(s scanner) (domain.ToolDefinition, error) {
	var (
		definition     domain.ToolDefinition
		headersJSON    string
		parametersJSON string
		strictInt      int
		activeInt      int
	)

	err := s.Scan(
		&definition.ID,
		&definition.Name,
		&definition.Description,
		&definition.Method,
		&definition.URL,
		&headersJSON,
		&parametersJSON,
		&strictInt,
		&activeInt,
	)
	if err != nil {
		return domain.ToolDefinition{}, err
	}

	if err := json.Unmarshal([]byte(headersJSON), &definition.Headers); err != nil {
		return domain.ToolDefinition{}, fmt.Errorf("unmarshal headers: %w", err)
	}
	if err := json.Unmarshal([]byte(parametersJSON), &definition.Parameters); err != nil {
		return domain.ToolDefinition{}, fmt.Errorf("unmarshal parameters: %w", err)
	}

	definition.Strict = strictInt == 1
	definition.Active = activeInt == 1
	return definition, nil
}

func (r *SQLiteToolDefinitionRepository) migrate() error {
	_, err := r.db.Exec(`
CREATE TABLE IF NOT EXISTS tool_definitions (
	internal_id INTEGER PRIMARY KEY AUTOINCREMENT,
	id TEXT NOT NULL UNIQUE,
	name TEXT NOT NULL,
	description TEXT NOT NULL DEFAULT '',
	method TEXT NOT NULL,
	url TEXT NOT NULL,
	headers_json TEXT NOT NULL,
	parameters_json TEXT NOT NULL,
	strict INTEGER NOT NULL DEFAULT 0,
	active INTEGER NOT NULL DEFAULT 1
);`)
	if err != nil {
		return fmt.Errorf("migrate tool_definitions: %w", err)
	}

	hasActiveColumn, err := r.hasColumn("tool_definitions", "active")
	if err != nil {
		return err
	}
	if !hasActiveColumn {
		if _, err := r.db.Exec("ALTER TABLE tool_definitions ADD COLUMN active INTEGER NOT NULL DEFAULT 1"); err != nil {
			return fmt.Errorf("add active column: %w", err)
		}
	}

	if _, err := r.db.Exec("UPDATE tool_definitions SET active = 1 WHERE active IS NULL"); err != nil {
		return fmt.Errorf("normalize active column: %w", err)
	}

	return nil
}

func (r *SQLiteToolDefinitionRepository) hasColumn(table string, column string) (bool, error) {
	rows, err := r.db.Query(fmt.Sprintf("PRAGMA table_info(%s)", table))
	if err != nil {
		return false, fmt.Errorf("table info query: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var (
			cid        int
			name       string
			columnType string
			notNull    int
			defaultVal sql.NullString
			pk         int
		)
		if err := rows.Scan(&cid, &name, &columnType, &notNull, &defaultVal, &pk); err != nil {
			return false, fmt.Errorf("table info scan: %w", err)
		}
		if name == column {
			return true, nil
		}
	}

	if err := rows.Err(); err != nil {
		return false, fmt.Errorf("table info rows: %w", err)
	}

	return false, nil
}

func ensureDir(dbPath string) error {
	dir := filepath.Dir(dbPath)
	if dir == "." {
		return nil
	}

	return mkdirAll(dir)
}

func boolToInt(value bool) int {
	if value {
		return 1
	}
	return 0
}
