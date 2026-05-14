package payloadschema

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestValidateNewsItemPayload_Valid(t *testing.T) {
	payload := json.RawMessage(`{
		"payload_version":"v1",
		"source":"hackernews",
		"source_item_id":"12345",
		"title":"Model release",
		"source_metadata":{
			"collection":"ai_news",
			"job_name":"openclaw-ai-daily",
			"job_run_id":"run_2026_02_14_001",
			"scraped_at":"2026-02-14T10:00:00Z",
			"score":42
		},
		"canonical_url":"https://example.com/story/12345",
		"published_at":"2026-02-13T14:00:00Z",
		"authors":["Alice","Bob"],
		"tags":["ai","release"]
	}`)

	item, err := ValidateNewsItemPayload(payload)
	if err != nil {
		t.Fatalf("expected payload to be valid, got error: %v", err)
	}

	if item.Source != "hackernews" {
		t.Fatalf("expected source=hackernews, got %q", item.Source)
	}
	if item.PayloadVersion != "v1" {
		t.Fatalf("expected payload_version=v1, got %q", item.PayloadVersion)
	}
}

func TestValidateNewsItemPayload_MissingRequired(t *testing.T) {
	payload := json.RawMessage(`{
		"payload_version":"v1",
		"source":"reddit",
		"title":"Missing source item id",
		"source_metadata":{
			"collection":"world_news",
			"job_name":"openclaw-world-daily",
			"job_run_id":"run_2026_02_14_002",
			"scraped_at":"2026-02-14T10:00:00Z"
		}
	}`)

	_, err := ValidateNewsItemPayload(payload)
	if err == nil {
		t.Fatalf("expected validation to fail for missing source_item_id")
	}
}

func TestValidateNewsItemPayload_WhitespaceTitle(t *testing.T) {
	payload := json.RawMessage(`{
		"payload_version":"v1",
		"source":"reddit",
		"source_item_id":"abc",
		"title":"   ",
		"source_metadata":{
			"collection":"world_news",
			"job_name":"openclaw-world-daily",
			"job_run_id":"run_2026_02_14_003",
			"scraped_at":"2026-02-14T10:00:00Z"
		}
	}`)

	_, err := ValidateNewsItemPayload(payload)
	if err == nil {
		t.Fatalf("expected validation to fail for whitespace-only title")
	}
	if !strings.Contains(err.Error(), "title must not be empty") {
		t.Fatalf("expected title semantic error, got: %v", err)
	}
}

func TestValidateNewsItemPayload_InvalidPublishedAt(t *testing.T) {
	payload := json.RawMessage(`{
		"payload_version":"v1",
		"source":"rss",
		"source_item_id":"id-1",
		"title":"Bad date",
		"published_at":"not-a-timestamp",
		"source_metadata":{
			"collection":"china_news",
			"job_name":"openclaw-china-daily",
			"job_run_id":"run_2026_02_14_004",
			"scraped_at":"2026-02-14T10:00:00Z"
		}
	}`)

	_, err := ValidateNewsItemPayload(payload)
	if err == nil {
		t.Fatalf("expected validation to fail for invalid published_at")
	}
}

func TestValidateNewsItemPayload_WithCollectionMetadata(t *testing.T) {
	payload := json.RawMessage(`{
		"payload_version":"v1",
		"source":"rss",
		"source_item_id":"id-collection-1",
		"title":"Tagged collection payload",
		"source_metadata":{
			"collection":"ai_news",
			"job_name":"openclaw-ai-daily",
			"job_run_id":"run_2026_02_14_001",
			"scraped_at":"2026-02-14T10:00:00Z"
		}
	}`)

	_, err := ValidateNewsItemPayload(payload)
	if err != nil {
		t.Fatalf("expected payload with collection metadata to be valid, got error: %v", err)
	}
}

func TestValidateNewsItemPayload_CollectionMustBeString(t *testing.T) {
	payload := json.RawMessage(`{
		"payload_version":"v1",
		"source":"rss",
		"source_item_id":"id-collection-2",
		"title":"Bad collection type",
		"source_metadata":{
			"collection":123,
			"job_name":"openclaw-ai-daily",
			"job_run_id":"run_2026_02_14_005",
			"scraped_at":"2026-02-14T10:00:00Z"
		}
	}`)

	_, err := ValidateNewsItemPayload(payload)
	if err == nil {
		t.Fatalf("expected validation to fail when source_metadata.collection is not a string")
	}
}

func TestValidateNewsItemPayload_MetadataInnerKeysRequired(t *testing.T) {
	payload := json.RawMessage(`{
		"payload_version":"v1",
		"source":"rss",
		"source_item_id":"id-collection-3",
		"title":"Missing metadata keys",
		"source_metadata":{
			"collection":"ai_news"
		}
	}`)

	_, err := ValidateNewsItemPayload(payload)
	if err == nil {
		t.Fatalf("expected validation to fail when source_metadata required keys are missing")
	}
}

func TestValidateNewsItemPayload_StrictJSONAndURIErrors(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name    string
		payload json.RawMessage
		want    string
	}{
		{name: "empty", payload: json.RawMessage(`  `), want: "payload is empty"},
		{name: "trailing", payload: json.RawMessage(`{} {}`), want: "trailing content"},
		{
			name: "blank source",
			payload: json.RawMessage(`{
				"payload_version":"v1",
				"source":" ",
				"source_item_id":"id-blank-source",
				"title":"Blank source",
				"source_metadata":{
					"collection":"ai_news",
					"job_name":"openclaw-ai-daily",
					"job_run_id":"run_2026_02_14_006",
					"scraped_at":"2026-02-14T10:00:00Z"
				}
			}`),
			want: "source must not be empty",
		},
		{
			name: "wrong payload version",
			payload: json.RawMessage(`{
				"payload_version":"v2",
				"source":"rss",
				"source_item_id":"id-version",
				"title":"Bad version",
				"source_metadata":{
					"collection":"ai_news",
					"job_name":"openclaw-ai-daily",
					"job_run_id":"run_2026_02_14_007",
					"scraped_at":"2026-02-14T10:00:00Z"
				}
			}`),
			want: "schema validation failed",
		},
		{
			name: "blank canonical URL",
			payload: json.RawMessage(`{
				"payload_version":"v1",
				"source":"rss",
				"source_item_id":"id-url",
				"title":"Blank URL",
				"canonical_url":" ",
				"source_metadata":{
					"collection":"ai_news",
					"job_name":"openclaw-ai-daily",
					"job_run_id":"run_2026_02_14_008",
					"scraped_at":"2026-02-14T10:00:00Z"
				}
			}`),
			want: "schema validation failed",
		},
	}

	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			_, err := ValidateNewsItemPayload(tt.payload)
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("ValidateNewsItemPayload() error = %v, want containing %q", err, tt.want)
			}
		})
	}
}

func TestValidateSemantics(t *testing.T) {
	valid := NewsItem{
		PayloadVersion: "v1",
		Source:         "rss",
		SourceItemID:   "source-1",
		Title:          "Story title",
	}
	canonicalURL := "https://example.com/story"
	imageURL := "https://example.com/image.png"
	publishedAt := "2026-02-14T10:00:00Z"

	tests := []struct {
		name string
		item *NewsItem
		want string
	}{
		{
			name: "nil",
			item: nil,
			want: "payload is nil",
		},
		{
			name: "blank source",
			item: withNewsItem(valid, func(item *NewsItem) {
				item.Source = " "
			}),
			want: "source must not be empty",
		},
		{
			name: "blank source item id",
			item: withNewsItem(valid, func(item *NewsItem) {
				item.SourceItemID = " "
			}),
			want: "source_item_id must not be empty",
		},
		{
			name: "blank title",
			item: withNewsItem(valid, func(item *NewsItem) {
				item.Title = " "
			}),
			want: "title must not be empty",
		},
		{
			name: "wrong payload version",
			item: withNewsItem(valid, func(item *NewsItem) {
				item.PayloadVersion = "v2"
			}),
			want: "payload_version must be v1",
		},
		{
			name: "valid URI fields and published time",
			item: withNewsItem(valid, func(item *NewsItem) {
				item.CanonicalURL = &canonicalURL
				item.ImageURL = &imageURL
				item.PublishedAt = &publishedAt
				item.Authors = []string{"Alice"}
				item.Tags = []string{"release"}
			}),
		},
		{
			name: "blank canonical URL",
			item: withNewsItem(valid, func(item *NewsItem) {
				blank := " "
				item.CanonicalURL = &blank
			}),
			want: "canonical_url must not be empty",
		},
		{
			name: "invalid image URL",
			item: withNewsItem(valid, func(item *NewsItem) {
				invalid := "://bad"
				item.ImageURL = &invalid
			}),
			want: "image_url is not a valid URI",
		},
		{
			name: "invalid published at",
			item: withNewsItem(valid, func(item *NewsItem) {
				invalid := "not-a-time"
				item.PublishedAt = &invalid
			}),
			want: "published_at must be RFC3339",
		},
		{
			name: "blank author",
			item: withNewsItem(valid, func(item *NewsItem) {
				item.Authors = []string{"Alice", " "}
			}),
			want: "authors[1] must not be empty",
		},
		{
			name: "blank tag",
			item: withNewsItem(valid, func(item *NewsItem) {
				item.Tags = []string{"ai", " "}
			}),
			want: "tags[1] must not be empty",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateSemantics(tt.item)
			if tt.want == "" {
				if err != nil {
					t.Fatalf("validateSemantics() error = %v", err)
				}
				return
			}
			if err == nil {
				t.Fatalf("validateSemantics() error = nil, want %q", tt.want)
			}
			if !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("validateSemantics() error = %q, want %q", err, tt.want)
			}
		})
	}
}

func withNewsItem(base NewsItem, mutate func(*NewsItem)) *NewsItem {
	item := base
	mutate(&item)
	return &item
}
