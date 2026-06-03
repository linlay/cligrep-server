package models

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestCLIReleaseAssetJSONIncludesID(t *testing.T) {
	payload, err := json.Marshal(CLIReleaseAsset{
		ID:          123,
		ReleaseID:   456,
		FileName:    "tool-darwin-arm64.tar.gz",
		DownloadURL: "https://downloads.example.com/tool-darwin-arm64.tar.gz",
	})
	if err != nil {
		t.Fatalf("marshal asset: %v", err)
	}

	body := string(payload)
	if !strings.Contains(body, `"id":123`) {
		t.Fatalf("expected asset id in JSON, got %s", body)
	}
	if strings.Contains(body, "releaseId") {
		t.Fatalf("expected release id to remain hidden, got %s", body)
	}
}
