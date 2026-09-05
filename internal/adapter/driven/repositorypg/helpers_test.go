package repositorypg

import (
	"testing"

	"uuid"

	"github.com/stretchr/testify/assert"

	"github.com/slashdevops/go-rest-api-service-template/internal/core/domain"
)

func TestGetFieldValue(t *testing.T) {
	tests := []struct {
		name     string
		prefix   string
		field    string
		fields   []string
		expected string
	}{
		{
			name:     "empty field",
			prefix:   "t.",
			field:    "",
			fields:   []string{"id", "name", "created_at"},
			expected: "",
		},
		{
			name:     "field found with exact match",
			prefix:   "t.",
			field:    "name",
			fields:   []string{"id", "name", "created_at"},
			expected: "t.name",
		},
		{
			name:     "field found with AS alias",
			prefix:   "t.",
			field:    "user_name",
			fields:   []string{"id", "name AS user_name", "created_at"},
			expected: "name AS user_name",
		},
		{
			name:     "field found with partial match",
			prefix:   "t.",
			field:    "id",
			fields:   []string{"user_id", "name", "created_at"},
			expected: "t.id",
		},
		{
			name:     "field not found",
			prefix:   "t.",
			field:    "email",
			fields:   []string{"id", "name", "created_at"},
			expected: "",
		},
		{
			name:     "empty prefix",
			prefix:   "",
			field:    "name",
			fields:   []string{"id", "name", "created_at"},
			expected: "name",
		},
		{
			name:     "multiple fields with AS alias",
			prefix:   "t.",
			field:    "display_name",
			fields:   []string{"id", "first_name AS first", "last_name AS display_name"},
			expected: "last_name AS display_name",
		},
		{
			name:     "non-matching prefix case sensitivity",
			prefix:   "t.",
			field:    "Name",
			fields:   []string{"id", "name", "created_at"},
			expected: "",
		},
		{
			name:     "field found with mixed case",
			prefix:   "t.",
			field:    "UserName",
			fields:   []string{"id", "name AS UserName", "created_at"},
			expected: "name AS UserName",
		},
		{
			name:     "AS in lowercase",
			prefix:   "t.",
			field:    "user_name",
			fields:   []string{"id", "name as user_name", "created_at"},
			expected: "name as user_name",
		},
		{
			name:     "AS in uppercase",
			prefix:   "t.",
			field:    "user_name",
			fields:   []string{"id", "name AS user_name", "created_at"},
			expected: "name AS user_name",
		},
		{
			name:     "AS in mixed case",
			prefix:   "t.",
			field:    "user_name",
			fields:   []string{"id", "name As user_name", "created_at"},
			expected: "name As user_name",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := getFieldValue(tt.prefix, tt.field, tt.fields)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestPrettyPrint(t *testing.T) {
	tests := []struct {
		name     string
		query    string
		args     []any
		expected string
	}{
		{
			name: "Simple query without args",
			query: `SELECT *
                  FROM users`,
			args:     nil,
			expected: "SELECT * FROM users",
		},
		{
			name: "Query with SQL comments",
			query: `SELECT *
                  -- This is a comment
                  FROM users`,
			args:     nil,
			expected: "SELECT * FROM users",
		},
		{
			name: "Query with multiple whitespaces and newlines",
			query: `SELECT *
                  FROM   users
                  WHERE  id  =  1`,
			args:     nil,
			expected: "SELECT * FROM users WHERE id = 1",
		},
		{
			name: "Query with string argument",
			query: `SELECT *
                  FROM users
                  WHERE name = $1`,
			args:     []any{"John"},
			expected: "SELECT * FROM users WHERE name = 'John'",
		},
		{
			name: "Query with multiple string arguments",
			query: `SELECT *
                  FROM users
                  WHERE name = $1 AND email = $2`,
			args:     []any{"John", "john@example.com"},
			expected: "SELECT * FROM users WHERE name = 'John' AND email = 'john@example.com'",
		},
		{
			name: "Query with non-string argument",
			query: `SELECT *
                  FROM users
                  WHERE id = $1`,
			args:     []any{123},
			expected: "SELECT * FROM users WHERE id = 123",
		},
		{
			name: "Query with mixed arguments",
			query: `SELECT *
                  FROM users
                  WHERE id = $1 AND name = $2`,
			args:     []any{123, "John"},
			expected: "SELECT * FROM users WHERE id = 123 AND name = 'John'",
		},
		{
			name: "Query with boolean argument",
			query: `SELECT *
                  FROM users
                  WHERE active = $1`,
			args:     []any{true},
			expected: "SELECT * FROM users WHERE active = true",
		},
		{
			name: "Query with UUID argument",
			query: `SELECT *
                  FROM users
                  WHERE id = $1`,
			args:     []any{"550e8400-e29b-41d4-a716-446655440000"},
			expected: "SELECT * FROM users WHERE id = '550e8400-e29b-41d4-a716-446655440000'",
		},
		{
			name: "Complex query with multiple arguments and comments",
			query: `SELECT u.id, u.name, u.email
                  -- Get users
                  FROM users u
                  -- Join with roles
                  JOIN user_roles ur ON u.id = ur.user_id
                  WHERE u.department = $1 AND ur.role = $2
                  -- Filter by active status
                  AND u.active = $3`,
			args:     []any{"Engineering", "Admin", true},
			expected: "SELECT u.id, u.name, u.email FROM users u JOIN user_roles ur ON u.id = ur.user_id WHERE u.department = 'Engineering' AND ur.role = 'Admin' AND u.active = true",
		},
		{
			name: "Query with multi-line comments",
			query: `SELECT *
                  -- This is a comment
                  -- This is another comment
                  FROM users`,
			args:     nil,
			expected: "SELECT * FROM users",
		},
		{
			name: "Query with more than 9 parameters",
			query: `INSERT INTO users (id, name, email, phone, address, city, state, country, zip, active)
                  VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)`,
			args:     []any{1, "John", "john@example.com", "1234567890", "123 Main St", "New York", "NY", "USA", "10001", true},
			expected: "INSERT INTO users (id, name, email, phone, address, city, state, country, zip, active) VALUES (1, 'John', 'john@example.com', '1234567890', '123 Main St', 'New York', 'NY', 'USA', '10001', true)",
		},
		{
			name:     "Empty query",
			query:    ``,
			args:     nil,
			expected: "",
		},
		{
			name: "Query with only comments",
			query: `-- This is a comment
                  -- This is another comment`,
			args:     nil,
			expected: "-- This is another comment",
		},
		{
			name: "Query with NULL argument",
			query: `SELECT *
                  FROM users
                  WHERE name = $1`,
			args:     []any{nil},
			expected: "SELECT * FROM users WHERE name = NULL",
		},
		{
			name: "Query with nil args slice",
			query: `SELECT *
                  FROM users
                  WHERE name = $1`,
			args:     nil,
			expected: "SELECT * FROM users WHERE name = $1",
		},
		{
			name: "Query with empty args slice",
			query: `SELECT *
                  FROM users
                  WHERE name = $1`,
			args:     []any{},
			expected: "SELECT * FROM users WHERE name = $1",
		},
		{
			name: "Query with boolean pointer (nil)",
			query: `SELECT *
                  FROM users
                  WHERE active = $1`,
			args:     []any{(*bool)(nil)},
			expected: "SELECT * FROM users WHERE active = NULL",
		},
		{
			name: "Query with boolean pointer (true)",
			query: `SELECT *
                  FROM users
                  WHERE active = $1`,
			args:     []any{domain.MakePointer(true)},
			expected: "SELECT * FROM users WHERE active = true",
		},
		{
			name: "Query with boolean pointer (false)",
			query: `SELECT *
                  FROM users
                  WHERE active = $1`,
			args:     []any{domain.MakePointer(false)},
			expected: "SELECT * FROM users WHERE active = false",
		},
		{
			name: "Query with string pointer (nil)",
			query: `SELECT *
                  FROM users
                  WHERE name = $1`,
			args:     []any{(*string)(nil)},
			expected: "SELECT * FROM users WHERE name = NULL",
		},
		{
			name: "Query with string pointer (value)",
			query: `SELECT *
                  FROM users
                  WHERE name = $1`,
			args:     []any{domain.MakePointer("John")},
			expected: "SELECT * FROM users WHERE name = 'John'",
		},
		{
			name: "Query with int pointer (nil)",
			query: `SELECT *
                  FROM users
                  WHERE id = $1`,
			args:     []any{(*int)(nil)},
			expected: "SELECT * FROM users WHERE id = NULL",
		},
		{
			name: "Query with int pointer (value)",
			query: `SELECT *
                  FROM users
                  WHERE id = $1`,
			args:     []any{domain.MakePointer(123)},
			expected: "SELECT * FROM users WHERE id = 123",
		},
		{
			name: "Query with mixed pointer and non-pointer args",
			query: `SELECT *
                  FROM users
                  WHERE id = $1 AND name = $2 AND active = $3`,
			args:     []any{123, domain.MakePointer("John"), domain.MakePointer(true)},
			expected: "SELECT * FROM users WHERE id = 123 AND name = 'John' AND active = true",
		},
		{
			name: "Query with float pointer (nil)",
			query: `SELECT *
                  FROM users
                  WHERE score = $1`,
			args:     []any{(*float64)(nil)},
			expected: "SELECT * FROM users WHERE score = NULL",
		},
		{
			name: "Query with float pointer (value)",
			query: `SELECT *
                  FROM users
                  WHERE score = $1`,
			args:     []any{domain.MakePointer(95.5)},
			expected: "SELECT * FROM users WHERE score = 95.5",
		},
		{
			name: "Query with UUID pointer (nil)",
			query: `SELECT *
                  FROM users
                  WHERE uuid = $1`,
			args:     []any{(*uuid.UUID)(nil)},
			expected: "SELECT * FROM users WHERE uuid = NULL",
		},
		{
			name: "Query with UUID pointer (value)",
			query: `SELECT *
                  FROM users
                  WHERE uuid = $1`,
			args:     []any{domain.MakePointer(uuid.MustParse("550e8400-e29b-41d4-a716-446655440000"))},
			expected: "SELECT * FROM users WHERE uuid = '550e8400-e29b-41d4-a716-446655440000'",
		},
		{
			name: "Query with various integer pointer types",
			query: `SELECT *
                  FROM users
                  WHERE id8 = $1 AND id16 = $2 AND id32 = $3 AND id64 = $4`,
			args:     []any{domain.MakePointer(int8(8)), domain.MakePointer(int16(16)), domain.MakePointer(int32(32)), domain.MakePointer(int64(64))},
			expected: "SELECT * FROM users WHERE id8 = 8 AND id16 = 16 AND id32 = 32 AND id64 = 64",
		},
		{
			name: "Query with various unsigned integer pointer types",
			query: `SELECT *
                  FROM users
                  WHERE uid = $1 AND uid8 = $2 AND uid16 = $3 AND uid32 = $4 AND uid64 = $5`,
			args:     []any{domain.MakePointer(uint(1)), domain.MakePointer(uint8(8)), domain.MakePointer(uint16(16)), domain.MakePointer(uint32(32)), domain.MakePointer(uint64(64))},
			expected: "SELECT * FROM users WHERE uid = 1 AND uid8 = 8 AND uid16 = 16 AND uid32 = 32 AND uid64 = 64",
		},
		{
			name: "Query with float32 pointer",
			query: `SELECT *
                  FROM users
                  WHERE score32 = $1`,
			args:     []any{domain.MakePointer(float32(3.14))},
			expected: "SELECT * FROM users WHERE score32 = 3.14",
		},
		{
			name: "Query with all nil pointers",
			query: `SELECT *
                  FROM users
                  WHERE name = $1 AND active = $2 AND id = $3 AND score = $4`,
			args: []any{
				(*string)(nil),
				(*bool)(nil),
				(*int)(nil),
				(*float64)(nil),
			},
			expected: "SELECT * FROM users WHERE name = NULL AND active = NULL AND id = NULL AND score = NULL",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := prettyPrint(tt.query, tt.args...)
			assert.Equal(t, tt.expected, result, "prettyPrint should transform the query correctly")
		})
	}
}

func TestBuildFieldSelection(t *testing.T) {
	tests := []struct {
		name            string
		sqlFieldsPrefix string
		fieldsArray     []string
		requestedFields string
		expected        string
	}{
		{
			name:            "no requested fields returns all fields",
			sqlFieldsPrefix: "t.",
			fieldsArray:     []string{"id", "name", "created_at"},
			requestedFields: "",
			expected:        "t.id, t.name, t.created_at",
		},
		{
			name:            "specific fields without id adds id and serial_id",
			sqlFieldsPrefix: "t.",
			fieldsArray:     []string{"id", "name", "created_at", "serial_id"},
			requestedFields: "name,created_at",
			expected:        "t.name, t.created_at, t.id, t.serial_id",
		},
		{
			name:            "id field included should not be duplicated",
			sqlFieldsPrefix: "t.",
			fieldsArray:     []string{"id", "name", "created_at", "serial_id"},
			requestedFields: "id,name",
			expected:        "t.id, t.name, t.serial_id",
		},
		{
			name:            "fields with AS aliases",
			sqlFieldsPrefix: "t.",
			fieldsArray:     []string{"id", "name AS user_name", "created_at", "serial_id"},
			requestedFields: "user_name",
			expected:        "name AS user_name, t.id, t.serial_id",
		},
		{
			name:            "id and serial_id both requested",
			sqlFieldsPrefix: "mt.",
			fieldsArray:     []string{"id", "name", "description", "serial_id"},
			requestedFields: "id,name,serial_id",
			expected:        "mt.id, mt.name, mt.serial_id",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := buildFieldSelection(tt.sqlFieldsPrefix, tt.fieldsArray, tt.requestedFields)
			assert.Equal(t, tt.expected, result, "buildFieldSelection should construct the field selection correctly")
		})
	}
}

func TestInjectPrefixToSortFields(t *testing.T) {
	tests := []struct {
		name          string
		prefix        string
		sortQuery     string
		allowedFields []string
		expected      string
	}{
		{
			name:          "simple ascending",
			prefix:        "idps.",
			sortQuery:     "name ASC",
			allowedFields: []string{"id", "name", "created_at"},
			expected:      "idps.name ASC",
		},
		{
			name:          "simple descending",
			prefix:        "idps.",
			sortQuery:     "name DESC",
			allowedFields: []string{"id", "name", "created_at"},
			expected:      "idps.name DESC",
		},
		{
			name:          "multiple fields",
			prefix:        "idps.",
			sortQuery:     "name ASC, id ASC",
			allowedFields: []string{"id", "name", "created_at"},
			expected:      "idps.name ASC, idps.id ASC",
		},
		{
			name:          "mixed case",
			prefix:        "idps.",
			sortQuery:     "name asc, id desc",
			allowedFields: []string{"id", "name", "created_at"},
			expected:      "idps.name asc, idps.id desc",
		},
		{
			name:          "no direction specified",
			prefix:        "idps.",
			sortQuery:     "name",
			allowedFields: []string{"id", "name", "created_at"},
			expected:      "idps.name",
		},
		{
			name:          "complex multiple",
			prefix:        "idps.",
			sortQuery:     "name ASC, created_at DESC, id ASC",
			allowedFields: []string{"id", "name", "created_at"},
			expected:      "idps.name ASC, idps.created_at DESC, idps.id ASC",
		},
		{
			name:          "field not in allowed list should be unchanged",
			prefix:        "idps.",
			sortQuery:     "unknown_field ASC, name DESC",
			allowedFields: []string{"id", "name", "created_at"},
			expected:      "unknown_field ASC, idps.name DESC",
		},
		{
			name:          "empty input",
			prefix:        "idps.",
			sortQuery:     "",
			allowedFields: []string{"id", "name", "created_at"},
			expected:      "",
		},
		{
			name:          "no prefix",
			prefix:        "",
			sortQuery:     "name ASC",
			allowedFields: []string{"id", "name", "created_at"},
			expected:      "name ASC",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := injectPrefixToSortFields(tt.prefix, tt.sortQuery, tt.allowedFields)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestInjectPrefixToFilterFields(t *testing.T) {
	tests := []struct {
		name          string
		prefix        string
		filter        string
		allowedFields []string
		expected      string
	}{
		{
			name:          "simple like",
			prefix:        "idps.",
			filter:        "name LIKE 'test%'",
			allowedFields: []string{"id", "name", "created_at"},
			expected:      "idps.name LIKE 'test%'",
		},
		{
			name:          "quoted string should not be prefixed",
			prefix:        "idps.",
			filter:        "name LIKE 'name is good'",
			allowedFields: []string{"id", "name", "created_at"},
			expected:      "idps.name LIKE 'name is good'",
		},
		{
			name:          "complex filter",
			prefix:        "idps.",
			filter:        "name = 'test' AND id != '123'",
			allowedFields: []string{"id", "name", "created_at"},
			expected:      "idps.name = 'test' AND idps.id != '123'",
		},
		{
			name:          "double quoted strings should not be prefixed",
			prefix:        "idps.",
			filter:        "name = \"test name\" AND id = '123'",
			allowedFields: []string{"id", "name", "created_at"},
			expected:      "idps.name = \"test name\" AND idps.id = '123'",
		},
		{
			name:          "field not in allowed list should be unchanged",
			prefix:        "idps.",
			filter:        "unknown_field = 'test' AND name = 'valid'",
			allowedFields: []string{"id", "name", "created_at"},
			expected:      "unknown_field = 'test' AND idps.name = 'valid'",
		},
		{
			name:          "empty input",
			prefix:        "idps.",
			filter:        "",
			allowedFields: []string{"id", "name", "created_at"},
			expected:      "",
		},
		{
			name:          "no prefix",
			prefix:        "",
			filter:        "name LIKE 'test%'",
			allowedFields: []string{"id", "name", "created_at"},
			expected:      "name LIKE 'test%'",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := injectPrefixToFields(tt.prefix, tt.filter, tt.allowedFields)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestBuildPaginationCriteria(t *testing.T) {
	testUUID := uuid.MustParse("550e8400-e29b-41d4-a716-446655440000")
	testSerial := int64(12345)

	tests := []struct {
		name           string
		tableAlias     string
		tokenDirection domain.TokenDirection
		id             uuid.UUID
		serial         int64
		filterQuery    string
		whereInQuery   bool
		expectedWhere  string
		expectedSort   string
	}{
		{
			name:           "no token - invalid direction with empty filter",
			tableAlias:     "users",
			tokenDirection: domain.TokenDirectionInvalid,
			id:             testUUID,
			serial:         testSerial,
			filterQuery:    "",
			whereInQuery:   false,
			expectedWhere:  "",
			expectedSort:   "users.serial_id DESC, users.id DESC",
		},
		{
			name:           "no token - with existing filter",
			tableAlias:     "users",
			tokenDirection: domain.TokenDirectionInvalid,
			id:             testUUID,
			serial:         testSerial,
			filterQuery:    "WHERE users.active = true",
			whereInQuery:   false,
			expectedWhere:  "WHERE users.active = true",
			expectedSort:   "users.serial_id DESC, users.id DESC",
		},
		{
			name:           "next token - empty filter",
			tableAlias:     "users",
			tokenDirection: domain.TokenDirectionNext,
			id:             testUUID,
			serial:         testSerial,
			filterQuery:    "",
			whereInQuery:   false,
			expectedWhere:  "\n\t\t\t\n\t\t\t\tWHERE (users.serial_id < '12345' OR (users.serial_id = '12345' AND users.id < '550e8400-e29b-41d4-a716-446655440000'))",
			expectedSort:   "users.serial_id DESC, users.id DESC",
		},
		{
			name:           "next token - with existing filter containing WHERE",
			tableAlias:     "users",
			tokenDirection: domain.TokenDirectionNext,
			id:             testUUID,
			serial:         testSerial,
			filterQuery:    "WHERE users.active = true",
			whereInQuery:   false,
			expectedWhere:  "\n\t\t\tWHERE users.active = true\n\t\t\t\tAND (users.serial_id < '12345' OR (users.serial_id = '12345' AND users.id < '550e8400-e29b-41d4-a716-446655440000'))",
			expectedSort:   "users.serial_id DESC, users.id DESC",
		},
		{
			name:           "next token - with existing filter containing AND",
			tableAlias:     "users",
			tokenDirection: domain.TokenDirectionNext,
			id:             testUUID,
			serial:         testSerial,
			filterQuery:    "users.active = true AND users.verified = true",
			whereInQuery:   false,
			expectedWhere:  "\n\t\t\tusers.active = true AND users.verified = true\n\t\t\t\tAND (users.serial_id < '12345' OR (users.serial_id = '12345' AND users.id < '550e8400-e29b-41d4-a716-446655440000'))",
			expectedSort:   "users.serial_id DESC, users.id DESC",
		},
		{
			name:           "next token - whereInQuery true",
			tableAlias:     "users",
			tokenDirection: domain.TokenDirectionNext,
			id:             testUUID,
			serial:         testSerial,
			filterQuery:    "users.active = true",
			whereInQuery:   true,
			expectedWhere:  "\n\t\t\tusers.active = true\n\t\t\t\tAND (users.serial_id < '12345' OR (users.serial_id = '12345' AND users.id < '550e8400-e29b-41d4-a716-446655440000'))",
			expectedSort:   "users.serial_id DESC, users.id DESC",
		},
		{
			name:           "prev token - empty filter",
			tableAlias:     "users",
			tokenDirection: domain.TokenDirectionPrev,
			id:             testUUID,
			serial:         testSerial,
			filterQuery:    "",
			whereInQuery:   false,
			expectedWhere:  "\n\t\t\t\n\t\t\t\tWHERE (users.serial_id > '12345' OR (users.serial_id = '12345' AND users.id > '550e8400-e29b-41d4-a716-446655440000'))",
			expectedSort:   "users.serial_id ASC, users.id ASC",
		},
		{
			name:           "prev token - with existing filter containing WHERE",
			tableAlias:     "users",
			tokenDirection: domain.TokenDirectionPrev,
			id:             testUUID,
			serial:         testSerial,
			filterQuery:    "WHERE users.active = true",
			whereInQuery:   false,
			expectedWhere:  "\n\t\t\tWHERE users.active = true\n\t\t\t\tAND (users.serial_id > '12345' OR (users.serial_id = '12345' AND users.id > '550e8400-e29b-41d4-a716-446655440000'))",
			expectedSort:   "users.serial_id ASC, users.id ASC",
		},
		{
			name:           "different table alias",
			tableAlias:     "projects",
			tokenDirection: domain.TokenDirectionNext,
			id:             testUUID,
			serial:         testSerial,
			filterQuery:    "",
			whereInQuery:   false,
			expectedWhere:  "\n\t\t\t\n\t\t\t\tWHERE (projects.serial_id < '12345' OR (projects.serial_id = '12345' AND projects.id < '550e8400-e29b-41d4-a716-446655440000'))",
			expectedSort:   "projects.serial_id DESC, projects.id DESC",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			whereClause, internalSort := buildPaginationCriteria(
				tt.tableAlias,
				tt.tokenDirection,
				tt.id,
				tt.serial,
				tt.filterQuery,
				tt.whereInQuery,
			)

			assert.Equal(t, tt.expectedWhere, string(whereClause))
			assert.Equal(t, tt.expectedSort, internalSort)
		})
	}
}
