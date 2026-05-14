package tools

import (
	"errors"
	"testing"
)

func TestSchemaValidate_OK(t *testing.T) {
	s := &Schema{
		Name:        "test_tool",
		Description: "A test tool",
		Fields: []FieldSpec{
			{Name: "query", Kind: KindString, Required: true, Description: "Search query"},
			{Name: "limit", Kind: KindNumber, Required: false, Description: "Result limit"},
		},
	}
	if err := s.Validate(); err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
}

func TestSchemaValidate_MissingName(t *testing.T) {
	s := &Schema{
		Description: "A test tool",
		Fields: []FieldSpec{
			{Name: "query", Kind: KindString, Required: true},
		},
	}
	err := s.Validate()
	if err == nil {
		t.Fatal("expected error for missing name")
	}
	if !errors.Is(err, ErrInvalidSchema) {
		t.Fatalf("expected ErrInvalidSchema, got %v", err)
	}
}

func TestSchemaValidate_DuplicateFields(t *testing.T) {
	s := &Schema{
		Name: "test_tool",
		Fields: []FieldSpec{
			{Name: "query", Kind: KindString, Required: true},
			{Name: "query", Kind: KindNumber, Required: false},
		},
	}
	err := s.Validate()
	if err == nil {
		t.Fatal("expected error for duplicate fields")
	}
	if !errors.Is(err, ErrInvalidSchema) {
		t.Fatalf("expected ErrInvalidSchema, got %v", err)
	}
}

func TestSchemaValidate_KindUnset(t *testing.T) {
	s := &Schema{
		Name: "test_tool",
		Fields: []FieldSpec{
			{Name: "query", Kind: "", Required: true},
		},
	}
	err := s.Validate()
	if err == nil {
		t.Fatal("expected error for unset kind")
	}
	if !errors.Is(err, ErrInvalidSchema) {
		t.Fatalf("expected ErrInvalidSchema, got %v", err)
	}
}

func TestValidateArgs_MissingRequired(t *testing.T) {
	s := &Schema{
		Name: "test_tool",
		Fields: []FieldSpec{
			{Name: "query", Kind: KindString, Required: true},
		},
	}
	err := s.ValidateArgs(map[string]interface{}{})
	if err == nil {
		t.Fatal("expected error for missing required arg")
	}
	if !errors.Is(err, ErrMissingRequiredArg) {
		t.Fatalf("expected ErrMissingRequiredArg, got %v", err)
	}
}

func TestValidateArgs_WrongTypeString(t *testing.T) {
	s := &Schema{
		Name: "test_tool",
		Fields: []FieldSpec{
			{Name: "query", Kind: KindString, Required: true},
		},
	}
	err := s.ValidateArgs(map[string]interface{}{"query": 123})
	if err == nil {
		t.Fatal("expected error for wrong type")
	}
	if !errors.Is(err, ErrInvalidArgType) {
		t.Fatalf("expected ErrInvalidArgType, got %v", err)
	}
}

func TestValidateArgs_WrongTypeNumber(t *testing.T) {
	s := &Schema{
		Name: "test_tool",
		Fields: []FieldSpec{
			{Name: "limit", Kind: KindNumber, Required: true},
		},
	}
	err := s.ValidateArgs(map[string]interface{}{"limit": "not a number"})
	if err == nil {
		t.Fatal("expected error for wrong type")
	}
	if !errors.Is(err, ErrInvalidArgType) {
		t.Fatalf("expected ErrInvalidArgType, got %v", err)
	}
}

func TestValidateArgs_WrongTypeBool(t *testing.T) {
	s := &Schema{
		Name: "test_tool",
		Fields: []FieldSpec{
			{Name: "enabled", Kind: KindBool, Required: true},
		},
	}
	err := s.ValidateArgs(map[string]interface{}{"enabled": "yes"})
	if err == nil {
		t.Fatal("expected error for wrong type")
	}
	if !errors.Is(err, ErrInvalidArgType) {
		t.Fatalf("expected ErrInvalidArgType, got %v", err)
	}
}

func TestValidateArgs_WrongTypeArray(t *testing.T) {
	s := &Schema{
		Name: "test_tool",
		Fields: []FieldSpec{
			{Name: "items", Kind: KindArray, Required: true},
		},
	}
	err := s.ValidateArgs(map[string]interface{}{"items": "not an array"})
	if err == nil {
		t.Fatal("expected error for wrong type")
	}
	if !errors.Is(err, ErrInvalidArgType) {
		t.Fatalf("expected ErrInvalidArgType, got %v", err)
	}
}

func TestValidateArgs_WrongTypeObject(t *testing.T) {
	s := &Schema{
		Name: "test_tool",
		Fields: []FieldSpec{
			{Name: "config", Kind: KindObject, Required: true},
		},
	}
	err := s.ValidateArgs(map[string]interface{}{"config": "not an object"})
	if err == nil {
		t.Fatal("expected error for wrong type")
	}
	if !errors.Is(err, ErrInvalidArgType) {
		t.Fatalf("expected ErrInvalidArgType, got %v", err)
	}
}

func TestValidateArgs_UnknownKey(t *testing.T) {
	s := &Schema{
		Name: "test_tool",
		Fields: []FieldSpec{
			{Name: "query", Kind: KindString, Required: true},
		},
	}
	err := s.ValidateArgs(map[string]interface{}{"query": "test", "unknown": "value"})
	if err == nil {
		t.Fatal("expected error for unknown key")
	}
	if !errors.Is(err, ErrUnknownArg) {
		t.Fatalf("expected ErrUnknownArg, got %v", err)
	}
}

func TestValidateArgs_AcceptsIntForNumber(t *testing.T) {
	s := &Schema{
		Name: "test_tool",
		Fields: []FieldSpec{
			{Name: "limit", Kind: KindNumber, Required: true},
		},
	}
	err := s.ValidateArgs(map[string]interface{}{"limit": int(42)})
	if err != nil {
		t.Fatalf("expected no error for int, got %v", err)
	}
}

func TestValidateArgs_AcceptsFloat64ForNumber(t *testing.T) {
	s := &Schema{
		Name: "test_tool",
		Fields: []FieldSpec{
			{Name: "limit", Kind: KindNumber, Required: true},
		},
	}
	err := s.ValidateArgs(map[string]interface{}{"limit": float64(42.5)})
	if err != nil {
		t.Fatalf("expected no error for float64, got %v", err)
	}
}

func TestSchema_ToOpenAIFunction_Shape(t *testing.T) {
	s := &Schema{
		Name:        "search_tool",
		Description: "Search for information",
		Fields: []FieldSpec{
			{Name: "query", Kind: KindString, Required: true, Description: "Search query"},
			{Name: "limit", Kind: KindNumber, Required: false, Description: "Result limit"},
			{Name: "verbose", Kind: KindBool, Required: false, Description: "Verbose output"},
		},
	}

	result := s.ToOpenAIFunction()

	// Check top-level structure
	if result["type"] != "function" {
		t.Fatalf("expected type='function', got %v", result["type"])
	}

	funcObj, ok := result["function"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected function to be map[string]interface{}, got %T", result["function"])
	}

	// Check function name and description
	if funcObj["name"] != "search_tool" {
		t.Fatalf("expected name='search_tool', got %v", funcObj["name"])
	}
	if funcObj["description"] != "Search for information" {
		t.Fatalf("expected description='Search for information', got %v", funcObj["description"])
	}

	// Check parameters structure
	params, ok := funcObj["parameters"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected parameters to be map[string]interface{}, got %T", funcObj["parameters"])
	}

	if params["type"] != "object" {
		t.Fatalf("expected parameters.type='object', got %v", params["type"])
	}

	// Check properties
	props, ok := params["properties"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected properties to be map[string]interface{}, got %T", params["properties"])
	}

	// Check query field (required string)
	queryProp, ok := props["query"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected query property to be map[string]interface{}, got %T", props["query"])
	}
	if queryProp["type"] != "string" {
		t.Fatalf("expected query.type='string', got %v", queryProp["type"])
	}
	if queryProp["description"] != "Search query" {
		t.Fatalf("expected query.description='Search query', got %v", queryProp["description"])
	}

	// Check limit field (optional number)
	limitProp, ok := props["limit"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected limit property to be map[string]interface{}, got %T", props["limit"])
	}
	if limitProp["type"] != "number" {
		t.Fatalf("expected limit.type='number', got %v", limitProp["type"])
	}

	// Check verbose field (optional bool)
	verboseProp, ok := props["verbose"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected verbose property to be map[string]interface{}, got %T", props["verbose"])
	}
	if verboseProp["type"] != "boolean" {
		t.Fatalf("expected verbose.type='boolean', got %v", verboseProp["type"])
	}

	// Check required array
	required, ok := params["required"].([]interface{})
	if !ok {
		t.Fatalf("expected required to be []interface{}, got %T", params["required"])
	}
	if len(required) != 1 {
		t.Fatalf("expected 1 required field, got %d", len(required))
	}
	if required[0] != "query" {
		t.Fatalf("expected required[0]='query', got %v", required[0])
	}
}
