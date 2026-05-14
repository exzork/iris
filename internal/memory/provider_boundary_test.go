package memory

import (
	"go/parser"
	"go/token"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// TestMemoryPackage_NoDirectProviderSDKImport enforces the architectural
// boundary rule that the memory package must not import provider SDKs
// directly. All LLM calls go through internal/llm and all embeddings go
// through internal/embedder. See doc.go for the full contract.
func TestMemoryPackage_NoDirectProviderSDKImport(t *testing.T) {
	disallowed := []string{
		"github.com/sashabaranov/go-openai",
		"github.com/openai/openai-go",
	}

	files, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatalf("glob .go files: %v", err)
	}
	if len(files) == 0 {
		t.Fatal("no .go files found in internal/memory (test must run from package dir)")
	}

	fs := token.NewFileSet()
	for _, f := range files {
		if strings.HasSuffix(f, "_test.go") {
			continue
		}
		node, err := parser.ParseFile(fs, f, nil, parser.ImportsOnly)
		if err != nil {
			t.Fatalf("parse %s: %v", f, err)
		}
		for _, imp := range node.Imports {
			path := strings.Trim(imp.Path.Value, `"`)
			for _, bad := range disallowed {
				if path == bad {
					t.Errorf("%s directly imports %s; memory package must go through internal/llm and internal/embedder", f, bad)
				}
			}
		}
	}
}

// TestMemoryPackage_DoesNotDependOnOpenAISDK is the transitive-dep backstop:
// even indirect deps of the memory package must not pull in a provider SDK.
// This catches regressions where a new internal package adds a forbidden dep
// that then flows into memory through an import chain.
func TestMemoryPackage_DoesNotDependOnOpenAISDK(t *testing.T) {
	out, err := exec.Command("go", "list", "-deps", ".").Output()
	if err != nil {
		t.Fatalf("go list failed: %v", err)
	}
	deps := string(out)
	forbidden := []string{
		"github.com/sashabaranov/go-openai",
		"github.com/openai/openai-go",
	}
	for _, f := range forbidden {
		if strings.Contains(deps, f) {
			t.Fatalf("memory package transitively depends on %s (use internal/embedder ONNX runtime instead)", f)
		}
	}
}
