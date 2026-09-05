package domain

import (
	"testing"
)

func TestValidateGoTemplate(t *testing.T) {
	tests := []struct {
		name           string
		templateStr    string
		requiredFields []string
		fieldName      string
		expectError    bool
		errorCode      string
	}{
		{
			name:           "valid_template_with_required_fields",
			templateStr:    "You are a helpful assistant. Question: {{.Question}}. Context: {{range .Contexts}}{{.}}{{end}}",
			requiredFields: []string{"{{.Question}}", "{{range .Contexts}}"},
			fieldName:      FieldSystemPrompt,
			expectError:    false,
		},
		{
			name:           "valid_template_with_spaces",
			templateStr:    "You are a helpful assistant. Question: {{ .Question }}. Context: {{ range .Contexts }}{{ . }}{{ end }}",
			requiredFields: []string{"{{.Question}}", "{{range .Contexts}}"},
			fieldName:      FieldSystemPrompt,
			expectError:    false,
		},
		{
			name:           "valid_template_mixed_spacing",
			templateStr:    "You are a helpful assistant. Question: {{.Question}}. Context: {{ range .Contexts }}{{.}}{{end}}",
			requiredFields: []string{"{{.Question}}", "{{range .Contexts}}"},
			fieldName:      FieldSystemPrompt,
			expectError:    false,
		},
		{
			name:           "missing_question_field",
			templateStr:    "You are a helpful assistant. Context: {{range .Contexts}}{{.}}{{end}}",
			requiredFields: []string{"{{.Question}}", "{{range .Contexts}}"},
			fieldName:      FieldSystemPrompt,
			expectError:    true,
			errorCode:      "MISSING_TEMPLATE_FIELD",
		},
		{
			name:           "missing_contexts_field",
			templateStr:    "You are a helpful assistant. Question: {{.Question}}",
			requiredFields: []string{"{{.Question}}", "{{range .Contexts}}"},
			fieldName:      FieldSystemPrompt,
			expectError:    true,
			errorCode:      "MISSING_TEMPLATE_FIELD",
		},
		{
			name:           "missing_both_fields",
			templateStr:    "You are a helpful assistant.",
			requiredFields: []string{"{{.Question}}", "{{range .Contexts}}"},
			fieldName:      FieldSystemPrompt,
			expectError:    true,
			errorCode:      "MISSING_TEMPLATE_FIELD",
		},
		{
			name:           "invalid_template_syntax",
			templateStr:    "You are a helpful assistant. Question: {{.Question}. Context: {{range .Contexts}}{{.}}{{end}}",
			requiredFields: []string{"{{.Question}}", "{{range .Contexts}}"},
			fieldName:      FieldSystemPrompt,
			expectError:    true,
			errorCode:      "INVALID_TEMPLATE_SYNTAX",
		},
		{
			name:           "empty_template",
			templateStr:    "",
			requiredFields: []string{"{{.Question}}", "{{range .Contexts}}"},
			fieldName:      FieldSystemPrompt,
			expectError:    false,
		},
		{
			name:           "complex_valid_template",
			templateStr:    "You are a helpful AI assistant.\n\n{{if .Contexts}}Here is the context:\n{{range .Contexts}}- {{.}}\n{{end}}\nBased on the context above, answer: {{.Question}}{{else}}Answer: {{.Question}}{{end}}",
			requiredFields: []string{"{{.Question}}", "{{range .Contexts}}"},
			fieldName:      FieldSystemPrompt,
			expectError:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateGoTemplate(tt.templateStr, tt.requiredFields, tt.fieldName)

			if tt.expectError {
				if err == nil {
					t.Errorf("expected error but got none")
					return
				}

				valErr, ok := err.(*ValidationError)
				if !ok {
					t.Errorf("expected ValidationError but got %T", err)
					return
				}

				if valErr.Code != tt.errorCode {
					t.Errorf("expected error code %s but got %s", tt.errorCode, valErr.Code)
				}

				if valErr.Field != tt.fieldName {
					t.Errorf("expected field %s but got %s", tt.fieldName, valErr.Field)
				}
			} else {
				if err != nil {
					t.Errorf("unexpected error: %v", err)
				}
			}
		})
	}
}
