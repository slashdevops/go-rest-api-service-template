package repositorypg

import (
	"encoding/json"
	"fmt"
	"html/template"
	"regexp"
	"slices"
	"sort"
	"strings"

	"uuid"

	"github.com/slashdevops/go-rest-api-service-template/internal/core/domain"
)

// prettyPrint removes comments and extra spaces from a query.
// It also replaces parameter placeholders ($1, $2, etc.) with their respective values.
func prettyPrint(query string, arg ...any) string {
	// Check for empty query
	if query == "" {
		return ""
	}

	ws := regexp.MustCompile(`\s+`)

	// Handle only comments case
	if strings.TrimSpace(regexp.MustCompile(`--.*`).ReplaceAllString(query, "")) == "" {
		// If the query contains only comments, return the last comment
		commentLines := regexp.MustCompile(`--.*`).FindAllString(query, -1)
		if len(commentLines) > 0 {
			return strings.TrimSpace(commentLines[len(commentLines)-1])
		}
		return ""
	}

	out := regexp.MustCompile(`--.*\n`).ReplaceAllString(query, "")
	out = strings.ReplaceAll(out, "\n", "")
	out = ws.ReplaceAllString(out, " ")
	out = strings.TrimSpace(out)

	if len(arg) > 0 {
		// Replace the pattern $1,$2,..., $n with the corresponding arguments
		re := regexp.MustCompile(`\$\d{1,2}`)

		for _, a := range arg {
			var placeholder string

			loc := re.FindStringIndex(out)
			if loc == nil {
				break
			}

			switch v := a.(type) {
			case nil:
				placeholder = "NULL"
			case string:
				placeholder = fmt.Sprintf("'%s'", v)
			case int, int64, int32, int16, int8, uint, uint64, uint32, uint16, uint8:
				placeholder = fmt.Sprintf("%v", v)
			case float64, float32:
				placeholder = fmt.Sprintf("%v", v)
			case bool:
				placeholder = fmt.Sprintf("%v", v)
			case uuid.UUID:
				placeholder = fmt.Sprintf("'%s'", v.String())
			case *bool:
				if v == nil {
					placeholder = "NULL"
				} else {
					placeholder = fmt.Sprintf("%v", *v)
				}
			case *string:
				if v == nil {
					placeholder = "NULL"
				} else {
					placeholder = fmt.Sprintf("'%s'", *v)
				}
			case *int:
				if v == nil {
					placeholder = "NULL"
				} else {
					placeholder = fmt.Sprintf("%v", *v)
				}
			case *int8:
				if v == nil {
					placeholder = "NULL"
				} else {
					placeholder = fmt.Sprintf("%v", *v)
				}
			case *int16:
				if v == nil {
					placeholder = "NULL"
				} else {
					placeholder = fmt.Sprintf("%v", *v)
				}
			case *int32:
				if v == nil {
					placeholder = "NULL"
				} else {
					placeholder = fmt.Sprintf("%v", *v)
				}
			case *int64:
				if v == nil {
					placeholder = "NULL"
				} else {
					placeholder = fmt.Sprintf("%v", *v)
				}
			case *uint:
				if v == nil {
					placeholder = "NULL"
				} else {
					placeholder = fmt.Sprintf("%v", *v)
				}
			case *uint8:
				if v == nil {
					placeholder = "NULL"
				} else {
					placeholder = fmt.Sprintf("%v", *v)
				}
			case *uint16:
				if v == nil {
					placeholder = "NULL"
				} else {
					placeholder = fmt.Sprintf("%v", *v)
				}
			case *uint32:
				if v == nil {
					placeholder = "NULL"
				} else {
					placeholder = fmt.Sprintf("%v", *v)
				}
			case *uint64:
				if v == nil {
					placeholder = "NULL"
				} else {
					placeholder = fmt.Sprintf("%v", *v)
				}
			case *float32:
				if v == nil {
					placeholder = "NULL"
				} else {
					placeholder = fmt.Sprintf("%v", *v)
				}
			case *float64:
				if v == nil {
					placeholder = "NULL"
				} else {
					placeholder = fmt.Sprintf("%v", *v)
				}
			case *uuid.UUID:
				if v == nil {
					placeholder = "NULL"
				} else {
					placeholder = fmt.Sprintf("'%s'", v.String())
				}
			case map[string]any:
				if jsonBytes, err := json.Marshal(v); err == nil {
					placeholder = fmt.Sprintf("'%s'", string(jsonBytes))
				} else {
					placeholder = "'{}'"
				}
			default:
				// Handle complex types safely to avoid %! formatting issues
				if jsonBytes, err := json.Marshal(v); err == nil {
					placeholder = fmt.Sprintf("'%s'", string(jsonBytes))
				} else {
					// Fallback to safe string representation
					placeholder = fmt.Sprintf("'%s'", strings.ReplaceAll(fmt.Sprintf("%v", v), "'", "''"))
				}
			}

			out = out[:loc[0]] + placeholder + out[loc[1]:]
		}
	}

	return out
}

// getFieldValue returns the field value from the given fields slice.
// It checks if the field is empty or if it exists in the fields slice.
// If it exists, it returns the field value.
// If it doesn't exist, it returns an empty string.
// It also checks for the prefix and adds it to the field value if necessary.
// The function is case-insensitive for the "AS" keyword.
// It also checks for the field name in the fields slice.
func getFieldValue(prefix, field string, fields []string) string {
	if field == "" {
		return ""
	}

	for _, f := range fields {
		if strings.HasSuffix(f, " AS "+field) ||
			strings.HasSuffix(f, " as "+field) ||
			strings.HasSuffix(f, " As "+field) ||
			strings.HasSuffix(f, " aS "+field) {
			return f
		} else if strings.Contains(f, field) {
			return prefix + field
		}
	}

	return ""
}

// buildFieldSelection constructs the field selection part of the SQL query based on
// the requested fields, ensuring that ID and SerialID are always included.
func buildFieldSelection(
	sqlFieldsPrefix string,
	fieldsArray []string,
	requestedFields string,
) string {
	var fieldsStr string
	for i, field := range fieldsArray {

		if strings.Contains(field, " AS ") {
			fieldsStr += field + ", "
		} else {
			fieldsStr += sqlFieldsPrefix + field + ", "
		}

		// if it is the last field, remove the last comma
		if i == len(fieldsArray)-1 {
			fieldsStr = strings.TrimSuffix(fieldsStr, ", ")
		}
	}

	// If no specific fields requested, return the full field list
	if requestedFields == "" {
		return fieldsStr
	}

	inputFields := strings.Split(requestedFields, ",")
	fields := make([]string, 0)
	var idIsPresent, serialIDIsPresent bool

	for _, field := range inputFields {
		field = strings.TrimSpace(field)

		// Check if the original field name is "id" or "serial_id" before getting the field value
		if field == "id" {
			idIsPresent = true
		}
		if field == "serial_id" {
			serialIDIsPresent = true
		}

		fieldValue := getFieldValue(sqlFieldsPrefix, field, fieldsArray)
		fields = append(fields, fieldValue)
	}

	// ID and SerialID are required for pagination
	if !idIsPresent {
		fields = append(fields, sqlFieldsPrefix+"id")
	}

	if !serialIDIsPresent {
		fields = append(fields, sqlFieldsPrefix+"serial_id")
	}
	return strings.Join(fields, ", ")
}

// buildPaginationCriteria constructs the SQL criteria for pagination based on token direction.
// It handles both next and previous pagination directions and returns the appropriate SQL WHERE clause
// and sort order to apply to the query.
// The function takes the table alias, token direction, ID, serial ID, filter query, and optional desired sort order as input parameters.
// It returns the WHERE clause and the internal sort order as a string.
// The function uses the template.HTML type to safely handle HTML content.
// It is important to note that this function does not perform any SQL injection prevention,
// so it should be used with caution and only with trusted input.
func buildPaginationCriteria(
	tableAlias string,
	tokenDirection domain.TokenDirection,
	id uuid.UUID,
	serial int64,
	filterQuery string,
	whereInQuery bool,
) (whereClause template.HTML, internalSort string) {
	// Default sort order (newest to oldest) - used for pagination token logic
	defaultSort := fmt.Sprintf("%s.serial_id DESC, %s.id DESC", tableAlias, tableAlias)

	// For keyset pagination to work correctly, we MUST use serial_id-based sorting
	// in the internal query (within the CTE), regardless of the external sort requested.
	// The external sort is applied after pagination to present results in the desired order.
	// This ensures consistent pagination behavior while respecting the user's sort preference.
	internalSort = defaultSort // If filter is empty, start with WHERE, otherwise AND
	// filterQueryJoiner := "WHERE"
	// if filterQuery != "" {
	// 	filterQueryJoiner = "AND"
	// }
	var filterQueryJoiner string
	if strings.Contains(filterQuery, "WHERE") {
		filterQueryJoiner = "AND"
	} else if strings.Contains(filterQuery, "AND") {
		filterQueryJoiner = "AND"
	} else if whereInQuery {
		filterQueryJoiner = "AND"
	} else {
		filterQueryJoiner = "WHERE"
	}

	// Handle token directions
	switch tokenDirection {
	case domain.TokenDirectionNext:
		// For next token, get records with lower serial ID (older records)
		// Logic: serial_id < current_serial_id OR (serial_id = current_serial_id AND id < current_id)
		internalSort = defaultSort
		whereClause = template.HTML(fmt.Sprintf(`
			%s
				%s (%s.serial_id < '%d' OR (%s.serial_id = '%d' AND %s.id < '%s'))`,
			filterQuery,
			filterQueryJoiner,
			tableAlias,
			serial,
			tableAlias,
			serial,
			tableAlias,
			id.String(),
		))

	case domain.TokenDirectionPrev:
		// For prev token, get records with higher serial ID (newer records)
		// Logic: serial_id > current_serial_id OR (serial_id = current_serial_id AND id > current_id)
		internalSort = fmt.Sprintf("%s.serial_id ASC, %s.id ASC", tableAlias, tableAlias)
		whereClause = template.HTML(fmt.Sprintf(`
			%s
				%s (%s.serial_id > '%d' OR (%s.serial_id = '%d' AND %s.id > '%s'))`,
			filterQuery,
			filterQueryJoiner,
			tableAlias,
			serial,
			tableAlias,
			serial,
			tableAlias,
			id.String(),
		))

	default:
		// No pagination token provided, just use the filter
		// internalSort was already set above based on the external sort preference
		whereClause = template.HTML(filterQuery)
	}

	return whereClause, internalSort
}

// injectPrefixToFields intelligently injects a prefix to specific fields in a filter query.
// It uses a regular expression that identifies and ignores matches within quoted strings
// to prevent incorrectly modifying string literals.
func injectPrefixToFields(prefix, filter string, allowedFields []string) string {
	if prefix == "" || filter == "" || len(allowedFields) == 0 {
		return filter
	}

	// To prevent ambiguous matches where one allowed field is a substring of another
	// (e.g., "id" and "user_id"), we sort the fields by length in descending order.
	// The regex engine will then try to match longer fields first.
	sort.Slice(allowedFields, func(i, j int) bool {
		return len(allowedFields[i]) > len(allowedFields[j])
	})

	escapedFields := make([]string, len(allowedFields))
	for i, field := range allowedFields {
		escapedFields[i] = regexp.QuoteMeta(field)
	}

	// This pattern has three alternatives, separated by the OR operator `|`:
	// 1. `'[^']*'`: Matches any content within a pair of single quotes.
	// 2. `"[^"]*"`: Matches any content within a pair of double quotes.
	// 3. `\b(field1|field2)\b`: Matches any of the allowed fields as whole words.
	// By placing the quote patterns first, the regex engine prioritizes matching them.
	fieldsPattern := `\b(` + strings.Join(escapedFields, "|") + `)\b`
	pattern := `'[^']*'|"[^"]*"|` + fieldsPattern

	re := regexp.MustCompile(pattern)

	// The callback function inspects each match to decide on the replacement.
	return re.ReplaceAllStringFunc(filter, func(match string) string {
		// Check if the match is a quoted literal (starts with ' or ").
		// If it is, we return it unchanged.
		if strings.HasPrefix(match, "'") || strings.HasPrefix(match, `"`) {
			return match
		}

		// Otherwise, the match must be one of our target fields.
		// We prepend the prefix and return the new string.
		return prefix + match
	})
}

// injectPrefixToSortFields intelligently injects a prefix to specific fields in a sort query.
// It handles sort expressions like "name ASC, id DESC" and converts them to "table.name ASC, table.id DESC".
// It uses a regular expression that identifies field names followed by optional sort direction.
func injectPrefixToSortFields(prefix, sortQuery string, allowedFields []string) string {
	if prefix == "" || sortQuery == "" || len(allowedFields) == 0 {
		return sortQuery
	}

	// To prevent ambiguous matches where one allowed field is a substring of another
	// (e.g., "id" and "user_id"), we sort the fields by length in descending order.
	// The regex engine will then try to match longer fields first.
	sort.Slice(allowedFields, func(i, j int) bool {
		return len(allowedFields[i]) > len(allowedFields[j])
	})

	escapedFields := make([]string, len(allowedFields))
	for i, field := range allowedFields {
		escapedFields[i] = regexp.QuoteMeta(field)
	}

	// Pattern to match field names optionally followed by ASC/DESC
	// This pattern matches: field_name followed by optional whitespace and optional ASC/DESC
	fieldsPattern := `\b(` + strings.Join(escapedFields, "|") + `)\b(\s+(ASC|DESC|asc|desc))?`

	re := regexp.MustCompile(fieldsPattern)

	// The callback function inspects each match to decide on the replacement.
	return re.ReplaceAllStringFunc(sortQuery, func(match string) string {
		// Split the match to separate field name from direction
		parts := strings.Fields(match)
		if len(parts) == 0 {
			return match
		}

		fieldName := parts[0]

		// Check if this field is in our allowed fields list
		if slices.Contains(allowedFields, fieldName) {
			// Reconstruct the match with prefix
			result := prefix + fieldName
			if len(parts) > 1 {
				result += " " + strings.Join(parts[1:], " ")
			}
			return result
		}

		// If field not found in allowed fields, return unchanged
		return match
	})
}
