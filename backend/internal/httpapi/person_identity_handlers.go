package httpapi

import (
	"errors"
	"strings"

	"github.com/labstack/echo/v4"

	"horse.fit/scoop/internal/db"
	"horse.fit/scoop/internal/globaltime"
)

type articlePersonIdentityRequest struct {
	IdentityRef string `json:"identity_ref"`
}

func (s *Server) handlePersonIdentities(c echo.Context) error {
	includeArchived := strings.EqualFold(strings.TrimSpace(c.QueryParam("include_archived")), "true")
	query := strings.TrimSpace(c.QueryParam("q"))
	identities, err := s.pool.ListPersonIdentities(c.Request().Context(), query, includeArchived, 50)
	if err != nil {
		s.logger.Error().Err(err).Msg("query person identities failed")
		return internalError(c, "Failed to load person identities")
	}
	return success(c, map[string]any{"items": identities})
}

func (s *Server) handleArticlePersonIdentities(c echo.Context) error {
	articleUUID := strings.TrimSpace(c.Param("article_uuid"))
	if articleUUID == "" {
		return failValidation(c, map[string]string{"article_uuid": "is required"})
	}
	identities, err := s.pool.ListPersonIdentitiesForArticleUUID(c.Request().Context(), articleUUID)
	if err != nil {
		s.logger.Error().Err(err).Str("article_uuid", articleUUID).Msg("query article person identities failed")
		return internalError(c, "Failed to load article person identities")
	}
	return success(c, map[string]any{"items": identities})
}

func (s *Server) handleAddArticlePersonIdentity(c echo.Context) error {
	articleUUID := strings.TrimSpace(c.Param("article_uuid"))
	if articleUUID == "" {
		return failValidation(c, map[string]string{"article_uuid": "is required"})
	}
	var req articlePersonIdentityRequest
	if err := decodeJSONBody(c, &req); err != nil {
		return failValidation(c, map[string]string{"body": err.Error()})
	}
	if _, err := db.ParseIdentityRef(req.IdentityRef); err != nil {
		return failValidation(c, map[string]string{"identity_ref": err.Error()})
	}

	principal, ok := principalFromContext(c)
	if !ok {
		return unauthorizedResponse(c)
	}
	actorUserID := principal.UserID
	identity, err := s.pool.AddArticlePersonIdentity(c.Request().Context(), articleUUID, req.IdentityRef, &actorUserID, globaltime.UTC())
	if err != nil {
		if errors.Is(err, db.ErrNoRows) {
			return failNotFound(c, "Article not found")
		}
		if msg := mutationValidationMessage(err); msg != "" {
			return failValidation(c, map[string]string{"identity_ref": msg})
		}
		s.logger.Error().Err(err).Str("article_uuid", articleUUID).Str("identity_ref", req.IdentityRef).Msg("add article person identity failed")
		return internalError(c, "Failed to add article person identity")
	}
	return success(c, map[string]any{"article_uuid": articleUUID, "person_identity": identity})
}

func (s *Server) handleRemoveArticlePersonIdentity(c echo.Context) error {
	articleUUID := strings.TrimSpace(c.Param("article_uuid"))
	if articleUUID == "" {
		return failValidation(c, map[string]string{"article_uuid": "is required"})
	}
	identityRefOrUUID := strings.TrimSpace(c.Param("person_identity"))
	if identityRefOrUUID == "" {
		return failValidation(c, map[string]string{"person_identity": "is required"})
	}

	principal, ok := principalFromContext(c)
	if !ok {
		return unauthorizedResponse(c)
	}
	actorUserID := principal.UserID
	if err := s.pool.RemoveArticlePersonIdentity(c.Request().Context(), articleUUID, identityRefOrUUID, &actorUserID); err != nil {
		if errors.Is(err, db.ErrNoRows) {
			return failNotFound(c, "Article or person identity not found")
		}
		if msg := mutationValidationMessage(err); msg != "" {
			return failValidation(c, map[string]string{"person_identity": msg})
		}
		s.logger.Error().Err(err).Str("article_uuid", articleUUID).Str("person_identity", identityRefOrUUID).Msg("remove article person identity failed")
		return internalError(c, "Failed to remove article person identity")
	}
	return success(c, map[string]any{"article_uuid": articleUUID, "person_identity": identityRefOrUUID})
}
