package httpapi

import (
	"strings"

	"github.com/labstack/echo/v4"

	"horse.fit/scoop/internal/db"
	"horse.fit/scoop/internal/globaltime"
)

type articleTagRequest struct {
	Tag string `json:"tag"`
}

func (s *Server) handleTags(c echo.Context) error {
	includeArchived := strings.EqualFold(strings.TrimSpace(c.QueryParam("include_archived")), "true")
	tags, err := s.pool.ListTags(c.Request().Context(), includeArchived)
	if err != nil {
		s.logger.Error().Err(err).Msg("query tags failed")
		return internalError(c, "Failed to load tags")
	}
	return success(c, map[string]any{"items": tags})
}

func (s *Server) handleAddArticleTag(c echo.Context) error {
	articleUUID, err := articleUUIDFromParam(c)
	if err != nil {
		return err
	}
	var req articleTagRequest
	if err := decodeJSONBody(c, &req); err != nil {
		return failValidation(c, map[string]string{"body": err.Error()})
	}
	tagSlug := db.NormalizeTagSlug(req.Tag)
	if err := db.ValidateTagSlug(tagSlug); err != nil {
		return failValidation(c, map[string]string{"tag": err.Error()})
	}

	actorUserID, ok := actorUserIDFromContext(c)
	if !ok {
		return nil
	}
	if err := s.pool.AddArticleTag(c.Request().Context(), articleUUID, tagSlug, actorUserID, globaltime.UTC()); err != nil {
		return s.handleArticleRelationMutationError(c, err, "Article or tag not found", "article_uuid", "tag", tagSlug, "add article tag failed", "Failed to add article tag")
	}
	return success(c, map[string]any{"article_uuid": articleUUID, "tag": tagSlug})
}

func (s *Server) handleRemoveArticleTag(c echo.Context) error {
	articleUUID, err := articleUUIDFromParam(c)
	if err != nil {
		return err
	}
	tagSlug := db.NormalizeTagSlug(c.Param("tag"))
	if err := db.ValidateTagSlug(tagSlug); err != nil {
		return failValidation(c, map[string]string{"tag": err.Error()})
	}

	actorUserID, ok := actorUserIDFromContext(c)
	if !ok {
		return nil
	}
	if err := s.pool.RemoveArticleTag(c.Request().Context(), articleUUID, tagSlug, actorUserID); err != nil {
		return s.handleArticleRelationMutationError(c, err, "Article or tag not found", "article_uuid", "tag", tagSlug, "remove article tag failed", "Failed to remove article tag")
	}
	return success(c, map[string]any{"article_uuid": articleUUID, "tag": tagSlug})
}

// mutate4go-manifest-begin
// {"version":1,"tested_at":"2026-07-23T14:09:44+08:00","module_hash":"07061fc3a6825ef72bd542fa92f4d79cb1987e7e4d97a6535633689c994b8e2c","functions":[{"id":"func/Server.handleTags","name":"Server.handleTags","line":16,"end_line":24,"hash":"0c9fd486bcac0c5d27b806ee3b42cead7693bd65fa77684295ddd90274dfe573"},{"id":"func/Server.handleAddArticleTag","name":"Server.handleAddArticleTag","line":26,"end_line":48,"hash":"f4238185ca337f932cde213a7341c5233404ac2bffec7d6b881e6f39ba015fe5"},{"id":"func/Server.handleRemoveArticleTag","name":"Server.handleRemoveArticleTag","line":50,"end_line":68,"hash":"7e757e6d4a2090768e2c6b30a7072abeac858fc3611d828bc100794fd2a94300"}]}
// mutate4go-manifest-end
