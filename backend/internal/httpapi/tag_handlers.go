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
