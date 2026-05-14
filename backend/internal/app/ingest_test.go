package app

import (
	"encoding/json"
	"testing"
)

func TestIngestRequestFromPayloadBuildsDomainRequest(t *testing.T) {
	t.Parallel()

	payload := json.RawMessage(`{"payload_version":"v1","source":"manual_cli","source_item_id":"manual-1","title":"manual ingest event","body_text":"body","canonical_url":"https://example.com/manual-1","published_at":"2026-05-14T12:30:00Z","language":"en","source_metadata":{"collection":"OpenClaw","job_name":"manual_cli","job_run_id":"manual-1","scraped_at":"2026-05-14T12:31:00Z"}}`)
	checkpoint := json.RawMessage(`{"cursor":"manual-1"}`)

	req, err := ingestRequestFromPayload(ingestCommandConfig{
		payloadJSON:      payload,
		checkpointJSON:   checkpoint,
		triggeredByTopic: "manual",
	})
	if err != nil {
		t.Fatalf("ingestRequestFromPayload() error = %v", err)
	}
	if req.Source != "manual_cli" || req.SourceItemID != "manual-1" || req.Collection != "openclaw" {
		t.Fatalf("unexpected request identity fields: %#v", req)
	}
	if req.SourceItemURL != "https://example.com/manual-1" {
		t.Fatalf("SourceItemURL = %q, want canonical URL", req.SourceItemURL)
	}
	if req.SourcePublishedAt == nil || req.SourcePublishedAt.Format("2006-01-02T15:04:05Z07:00") != "2026-05-14T12:30:00Z" {
		t.Fatalf("SourcePublishedAt = %v, want payload published_at", req.SourcePublishedAt)
	}
	if string(req.RawPayload) != string(payload) || string(req.CursorCheckpoint) != string(checkpoint) {
		t.Fatalf("raw payload/checkpoint not preserved")
	}
}

func TestIngestRequestFromPayloadRejectsMissingCollection(t *testing.T) {
	t.Parallel()

	payload := json.RawMessage(`{"payload_version":"v1","source":"manual_cli","source_item_id":"manual-1","title":"manual ingest event","body_text":"body","published_at":"2026-05-14T12:30:00Z","language":"en","source_metadata":{"job_name":"manual_cli","job_run_id":"manual-1","scraped_at":"2026-05-14T12:31:00Z"}}`)
	if _, err := ingestRequestFromPayload(ingestCommandConfig{payloadJSON: payload}); err == nil {
		t.Fatalf("ingestRequestFromPayload() error = nil, want missing collection error")
	}
}
