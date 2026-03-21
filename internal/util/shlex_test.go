package util

import "testing"

func TestSplitLine(t *testing.T) {
	tokens, err := SplitLine(`create python "hello world"`)
	if err != nil {
		t.Fatalf("SplitLine returned error: %v", err)
	}

	want := []string{"create", "python", "hello world"}
	if len(tokens) != len(want) {
		t.Fatalf("token count mismatch: got %d want %d", len(tokens), len(want))
	}

	for index := range want {
		if tokens[index] != want[index] {
			t.Fatalf("token %d mismatch: got %q want %q", index, tokens[index], want[index])
		}
	}
}

func TestContainsForbiddenOperator(t *testing.T) {
	if !ContainsForbiddenOperator("grep foo | sort") {
		t.Fatal("expected pipe to be forbidden")
	}
	if ContainsForbiddenOperator(`create python "foo | bar"`) {
		t.Fatal("expected quoted pipe to be allowed")
	}
}
