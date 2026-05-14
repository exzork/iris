package escalate

import (
	"context"
	"testing"

	"github.com/eko/iris-bot/internal/tools"
)

func TestEscalateSchemaShape(t *testing.T) {
	tool := New()
	schema := tool.Schema()

	if schema.Name != Name {
		t.Errorf("schema.Name = %q, want %q", schema.Name, Name)
	}

	if schema.Description == "" {
		t.Error("schema.Description is empty")
	}

	if len(schema.Fields) != 1 {
		t.Errorf("len(schema.Fields) = %d, want 1", len(schema.Fields))
	}

	field := schema.Fields[0]
	if field.Name != "reason" {
		t.Errorf("field.Name = %q, want %q", field.Name, "reason")
	}
	if field.Kind != tools.KindString {
		t.Errorf("field.Kind = %v, want %v", field.Kind, tools.KindString)
	}
	if !field.Required {
		t.Error("field.Required = false, want true")
	}
}

func TestEscalateRun_ReturnsMarker(t *testing.T) {
	tool := New()
	ctx := context.Background()

	reason := "needs deep lore analysis"
	result, err := tool.Run(ctx, map[string]interface{}{"reason": reason})

	if err != nil {
		t.Fatalf("Run() error = %v, want nil", err)
	}

	expected := MarkerPrefix + reason
	if result != expected {
		t.Errorf("Run() = %q, want %q", result, expected)
	}
}

func TestEscalateRun_MissingReason_Errors(t *testing.T) {
	tool := New()
	ctx := context.Background()

	_, err := tool.Run(ctx, map[string]interface{}{})

	if err == nil {
		t.Error("Run() error = nil, want error")
	}
}

func TestEscalateRun_EmptyReason_Errors(t *testing.T) {
	tool := New()
	ctx := context.Background()

	_, err := tool.Run(ctx, map[string]interface{}{"reason": ""})

	if err == nil {
		t.Error("Run() error = nil, want error")
	}
}

func TestEscalateRun_WrongType_Errors(t *testing.T) {
	tool := New()
	ctx := context.Background()

	_, err := tool.Run(ctx, map[string]interface{}{"reason": 123})

	if err == nil {
		t.Error("Run() error = nil, want error")
	}
}
