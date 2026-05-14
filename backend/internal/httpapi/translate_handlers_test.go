package httpapi

import (
	"context"
	"fmt"
	"net/http"
	"testing"

	"github.com/rs/zerolog"

	"horse.fit/scoop/internal/translation"
)

type fakeTranslationService struct {
	defaultProvider string
	storyCalls      []string
	articleCalls    []string
	nextStats       translation.RunStats
	nextErr         error
}

func (s *fakeTranslationService) DefaultProvider() string {
	if s.defaultProvider == "" {
		return "local"
	}
	return s.defaultProvider
}

func (s *fakeTranslationService) TranslateStoryByUUID(_ context.Context, storyUUID string, _ translation.RunOptions) (translation.RunStats, error) {
	s.storyCalls = append(s.storyCalls, storyUUID)
	return s.nextStats, s.nextErr
}

func (s *fakeTranslationService) TranslateArticleByUUID(_ context.Context, articleUUID string, _ translation.RunOptions) (translation.RunStats, error) {
	s.articleCalls = append(s.articleCalls, articleUUID)
	return s.nextStats, s.nextErr
}

func (s *fakeTranslationService) TranslateCollection(context.Context, string, translation.CollectionRunOptions) (translation.RunStats, error) {
	return s.nextStats, s.nextErr
}

func (s *fakeTranslationService) ListStoryTranslationsByUUID(context.Context, string) ([]translation.CachedTranslation, error) {
	return nil, s.nextErr
}

func TestHandleTranslateTranslatesStory(t *testing.T) {
	t.Parallel()

	service := &fakeTranslationService{nextStats: translation.RunStats{Total: 1, Translated: 1}}
	server := &Server{logger: zerolog.Nop(), translationService: service}
	_, c, rec := newJSONContext(http.MethodPost, "/api/v1/translate", `{"story_uuid":"story-1","target_lang":"EN-us"}`)

	if err := server.handleTranslate(c); err != nil {
		t.Fatalf("handleTranslate() error = %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", rec.Code, rec.Body.String())
	}
	if len(service.storyCalls) != 1 || service.storyCalls[0] != "story-1" {
		t.Fatalf("story calls = %#v", service.storyCalls)
	}
}

func TestHandleTranslateTranslatesArticleWithProvider(t *testing.T) {
	t.Parallel()

	service := &fakeTranslationService{nextStats: translation.RunStats{Total: 1, Cached: 1}}
	server := &Server{logger: zerolog.Nop(), translationService: service}
	_, c, rec := newJSONContext(http.MethodPost, "/api/v1/translate", `{"article_uuid":"article-1","target_lang":"zh","provider":"local"}`)

	if err := server.handleTranslate(c); err != nil {
		t.Fatalf("handleTranslate() error = %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", rec.Code, rec.Body.String())
	}
	if len(service.articleCalls) != 1 || service.articleCalls[0] != "article-1" {
		t.Fatalf("article calls = %#v", service.articleCalls)
	}
}

func TestHandleTranslateValidatesTarget(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		body string
	}{
		{name: "missing target", body: `{"target_lang":"en"}`},
		{name: "two targets", body: `{"story_uuid":"story-1","article_uuid":"article-1","target_lang":"en"}`},
		{name: "missing lang", body: `{"story_uuid":"story-1"}`},
		{name: "unknown field", body: `{"story_uuid":"story-1","target_lang":"en","extra":true}`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			service := &fakeTranslationService{}
			server := &Server{logger: zerolog.Nop(), translationService: service}
			_, c, rec := newJSONContext(http.MethodPost, "/api/v1/translate", tt.body)
			if err := server.handleTranslate(c); err != nil {
				t.Fatalf("handleTranslate() error = %v", err)
			}
			if rec.Code != http.StatusBadRequest {
				t.Fatalf("status = %d body=%s", rec.Code, rec.Body.String())
			}
		})
	}
}

func TestHandleTranslateMapsServiceErrors(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		err  error
		code int
	}{
		{name: "story not found", err: translation.ErrStoryNotFound, code: http.StatusNotFound},
		{name: "article not found", err: translation.ErrArticleNotFound, code: http.StatusNotFound},
		{name: "disabled", err: translation.ErrTranslationDisabled, code: http.StatusBadRequest},
		{name: "provider", err: fmt.Errorf("provider local not registered"), code: http.StatusBadRequest},
		{name: "unexpected", err: fmt.Errorf("boom"), code: http.StatusInternalServerError},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			service := &fakeTranslationService{nextErr: tt.err}
			server := &Server{logger: zerolog.Nop(), translationService: service}
			_, c, rec := newJSONContext(http.MethodPost, "/api/v1/translate", `{"story_uuid":"story-1","target_lang":"en"}`)
			if err := server.handleTranslate(c); err != nil {
				t.Fatalf("handleTranslate() error = %v", err)
			}
			if rec.Code != tt.code {
				t.Fatalf("status = %d body=%s", rec.Code, rec.Body.String())
			}
		})
	}
}
