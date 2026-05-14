package tools

import (
	"fmt"
)

type Kind string

const (
	KindString Kind = "string"
	KindNumber Kind = "number"
	KindBool   Kind = "bool"
	KindObject Kind = "object"
	KindArray  Kind = "array"
)

type FieldSpec struct {
	Name        string
	Kind        Kind
	Required    bool
	Description string
}

type Schema struct {
	Name        string
	Description string
	Fields      []FieldSpec
}

// Validate checks the schema itself for correctness.
func (s *Schema) Validate() error {
	if s.Name == "" {
		return fmt.Errorf("%w: schema name is required", ErrInvalidSchema)
	}

	seen := make(map[string]bool)
	for _, field := range s.Fields {
		if field.Name == "" {
			return fmt.Errorf("%w: field name is required", ErrInvalidSchema)
		}
		if seen[field.Name] {
			return fmt.Errorf("%w: duplicate field %q", ErrInvalidSchema, field.Name)
		}
		seen[field.Name] = true

		if field.Kind == "" {
			return fmt.Errorf("%w: field %q kind is required", ErrInvalidSchema, field.Name)
		}

		switch field.Kind {
		case KindString, KindNumber, KindBool, KindObject, KindArray:
		default:
			return fmt.Errorf("%w: field %q has invalid kind %q", ErrInvalidSchema, field.Name, field.Kind)
		}
	}

	return nil
}

// ValidateArgs checks that provided arguments match the schema.
func (s *Schema) ValidateArgs(args map[string]interface{}) error {
	// Check for required fields and unknown keys
	provided := make(map[string]bool)
	for key := range args {
		provided[key] = true
	}

	// Check required fields are present
	for _, field := range s.Fields {
		if field.Required && !provided[field.Name] {
			return fmt.Errorf("%w: %q", ErrMissingRequiredArg, field.Name)
		}
	}

	// Check for unknown keys
	schemaFields := make(map[string]FieldSpec)
	for _, field := range s.Fields {
		schemaFields[field.Name] = field
	}
	for key := range args {
		if _, exists := schemaFields[key]; !exists {
			return fmt.Errorf("%w: %q", ErrUnknownArg, key)
		}
	}

	// Validate types of provided arguments
	for key, value := range args {
		field := schemaFields[key]
		if !s.validateType(field.Kind, value) {
			return fmt.Errorf("%w: field %q expected %s but got %T", ErrInvalidArgType, key, field.Kind, value)
		}
	}

	return nil
}

func (s *Schema) validateType(kind Kind, value interface{}) bool {
	switch kind {
	case KindString:
		_, ok := value.(string)
		return ok
	case KindNumber:
		switch value.(type) {
		case float64, int, int32, int64:
			return true
		}
		return false
	case KindBool:
		_, ok := value.(bool)
		return ok
	case KindArray:
		_, ok := value.([]interface{})
		return ok
	case KindObject:
		_, ok := value.(map[string]interface{})
		return ok
	}
	return false
}

// ToOpenAIFunction converts the schema to OpenAI function-calling format.
func (s *Schema) ToOpenAIFunction() map[string]interface{} {
	properties := make(map[string]interface{})
	required := make([]interface{}, 0)

	for _, field := range s.Fields {
		openaiType := s.kindToOpenAIType(field.Kind)
		properties[field.Name] = map[string]interface{}{
			"type":        openaiType,
			"description": field.Description,
		}
		if field.Required {
			required = append(required, field.Name)
		}
	}

	parameters := map[string]interface{}{
		"type":       "object",
		"properties": properties,
		"required":   required,
	}

	return map[string]interface{}{
		"type": "function",
		"function": map[string]interface{}{
			"name":        s.Name,
			"description": s.Description,
			"parameters":  parameters,
		},
	}
}

func (s *Schema) kindToOpenAIType(kind Kind) string {
	switch kind {
	case KindString:
		return "string"
	case KindNumber:
		return "number"
	case KindBool:
		return "boolean"
	case KindObject:
		return "object"
	case KindArray:
		return "array"
	default:
		return "string"
	}
}
