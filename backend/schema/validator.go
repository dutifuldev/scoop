package payloadschema

import (
	"bytes"
	_ "embed"
	"encoding/json"
	"fmt"
	"io"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/santhosh-tekuri/jsonschema/v5"
)

//go:embed news_item.schema.json
var newsItemSchemaJSON string

type NewsItem struct {
	PayloadVersion string         `json:"payload_version"`
	Source         string         `json:"source"`
	SourceItemID   string         `json:"source_item_id"`
	Title          string         `json:"title"`
	SourceMetadata map[string]any `json:"source_metadata"`
	CanonicalURL   *string        `json:"canonical_url,omitempty"`
	PublishedAt    *string        `json:"published_at,omitempty"`
	BodyText       *string        `json:"body_text,omitempty"`
	Language       *string        `json:"language,omitempty"`
	SourceDomain   *string        `json:"source_domain,omitempty"`
	Authors        []string       `json:"authors,omitempty"`
	Tags           []string       `json:"tags,omitempty"`
	ImageURL       *string        `json:"image_url,omitempty"`
}

var (
	compileOnce       sync.Once
	compiledSchema    *jsonschema.Schema
	compiledSchemaErr error
)

func ValidateNewsItemPayload(payload json.RawMessage) (*NewsItem, error) {
	value, err := decodeStrictJSON(payload)
	if err != nil {
		return nil, fmt.Errorf("decode payload JSON: %w", err)
	}

	schema, err := loadSchema()
	if err != nil {
		return nil, fmt.Errorf("load schema: %w", err)
	}

	if err := schema.Validate(value); err != nil {
		return nil, fmt.Errorf("schema validation failed: %w", err)
	}

	normalized, err := json.Marshal(value)
	if err != nil {
		return nil, fmt.Errorf("normalize payload JSON: %w", err)
	}

	var item NewsItem
	if err := json.Unmarshal(normalized, &item); err != nil {
		return nil, fmt.Errorf("unmarshal payload: %w", err)
	}

	if err := validateSemantics(&item); err != nil {
		return nil, err
	}

	return &item, nil
}

func loadSchema() (*jsonschema.Schema, error) {
	compileOnce.Do(func() {
		compiler := jsonschema.NewCompiler()
		compiler.Draft = jsonschema.Draft2020
		compiler.AssertFormat = true

		if err := compiler.AddResource("news_item.schema.json", strings.NewReader(newsItemSchemaJSON)); err != nil {
			compiledSchemaErr = fmt.Errorf("add schema resource: %w", err)
			return
		}

		schema, err := compiler.Compile("news_item.schema.json")
		if err != nil {
			compiledSchemaErr = fmt.Errorf("compile schema: %w", err)
			return
		}

		compiledSchema = schema
	})

	if compiledSchemaErr != nil {
		return nil, compiledSchemaErr
	}
	if compiledSchema == nil {
		return nil, fmt.Errorf("schema not initialized")
	}
	return compiledSchema, nil
}

func decodeStrictJSON(raw []byte) (any, error) {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 {
		return nil, fmt.Errorf("payload is empty")
	}

	decoder := json.NewDecoder(bytes.NewReader(trimmed))
	decoder.UseNumber()

	var value any
	if err := decoder.Decode(&value); err != nil {
		return nil, err
	}

	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		return nil, fmt.Errorf("payload contains trailing content")
	}

	return value, nil
}

func validateSemantics(item *NewsItem) error {
	if item == nil {
		return fmt.Errorf("payload is nil")
	}

	checks := []func(*NewsItem) error{
		validateRequiredNewsItemFields,
		validateOptionalNewsItemURIs,
		validateOptionalPublishedAt,
		validateAuthors,
		validateTags,
	}
	for _, check := range checks {
		if err := check(item); err != nil {
			return err
		}
	}

	return nil
}

func validateRequiredNewsItemFields(item *NewsItem) error {
	required := []struct {
		field string
		value string
	}{
		{field: "source", value: item.Source},
		{field: "source_item_id", value: item.SourceItemID},
		{field: "title", value: item.Title},
		{field: "payload_version", value: item.PayloadVersion},
	}
	for _, entry := range required {
		if strings.TrimSpace(entry.value) == "" {
			return fmt.Errorf("%s must not be empty", entry.field)
		}
	}
	if strings.TrimSpace(item.PayloadVersion) != "v1" {
		return fmt.Errorf("payload_version must be v1")
	}
	return nil
}

func validateOptionalNewsItemURIs(item *NewsItem) error {
	if item.CanonicalURL != nil {
		if err := validateURI("canonical_url", *item.CanonicalURL); err != nil {
			return err
		}
	}
	if item.ImageURL != nil {
		if err := validateURI("image_url", *item.ImageURL); err != nil {
			return err
		}
	}
	return nil
}

func validateOptionalPublishedAt(item *NewsItem) error {
	if item.PublishedAt == nil {
		return nil
	}
	if _, err := time.Parse(time.RFC3339, strings.TrimSpace(*item.PublishedAt)); err != nil {
		return fmt.Errorf("published_at must be RFC3339: %w", err)
	}
	return nil
}

func validateAuthors(item *NewsItem) error {
	return validateNonEmptyStringList("authors", item.Authors)
}

func validateTags(item *NewsItem) error {
	return validateNonEmptyStringList("tags", item.Tags)
}

func validateNonEmptyStringList(field string, values []string) error {
	for i, value := range values {
		if strings.TrimSpace(value) == "" {
			return fmt.Errorf("%s[%d] must not be empty", field, i)
		}
	}
	return nil
}

func validateURI(fieldName, value string) error {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return fmt.Errorf("%s must not be empty", fieldName)
	}
	if _, err := url.ParseRequestURI(trimmed); err != nil {
		return fmt.Errorf("%s is not a valid URI: %w", fieldName, err)
	}
	return nil
}
