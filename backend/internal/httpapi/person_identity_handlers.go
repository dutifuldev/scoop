package httpapi

import (
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
	articleUUID, err := articleUUIDFromParam(c)
	if err != nil {
		return err
	}
	identities, err := s.pool.ListPersonIdentitiesForArticleUUID(c.Request().Context(), articleUUID)
	if err != nil {
		s.logger.Error().Err(err).Str("article_uuid", articleUUID).Msg("query article person identities failed")
		return internalError(c, "Failed to load article person identities")
	}
	return success(c, map[string]any{"items": identities})
}

func (s *Server) handleAddArticlePersonIdentity(c echo.Context) error {
	articleUUID, err := articleUUIDFromParam(c)
	if err != nil {
		return err
	}
	var req articlePersonIdentityRequest
	if err := decodeJSONBody(c, &req); err != nil {
		return failValidation(c, map[string]string{"body": err.Error()})
	}
	if _, err := db.ParseIdentityRef(req.IdentityRef); err != nil {
		return failValidation(c, map[string]string{"identity_ref": err.Error()})
	}

	actorUserID, ok := actorUserIDFromContext(c)
	if !ok {
		return nil
	}
	identity, err := s.pool.AddArticlePersonIdentity(c.Request().Context(), articleUUID, req.IdentityRef, actorUserID, globaltime.UTC())
	if err != nil {
		return s.handleArticleRelationMutationError(c, err, "Article not found", "identity_ref", "identity_ref", req.IdentityRef, "add article person identity failed", "Failed to add article person identity")
	}
	return success(c, map[string]any{"article_uuid": articleUUID, "person_identity": identity})
}

func (s *Server) handleRemoveArticlePersonIdentity(c echo.Context) error {
	articleUUID, err := articleUUIDFromParam(c)
	if err != nil {
		return err
	}
	identityRefOrUUID := strings.TrimSpace(c.Param("person_identity"))
	if identityRefOrUUID == "" {
		return failValidation(c, map[string]string{"person_identity": "is required"})
	}

	actorUserID, ok := actorUserIDFromContext(c)
	if !ok {
		return nil
	}
	if err := s.pool.RemoveArticlePersonIdentity(c.Request().Context(), articleUUID, identityRefOrUUID, actorUserID); err != nil {
		return s.handleArticleRelationMutationError(c, err, "Article or person identity not found", "person_identity", "person_identity", identityRefOrUUID, "remove article person identity failed", "Failed to remove article person identity")
	}
	return success(c, map[string]any{"article_uuid": articleUUID, "person_identity": identityRefOrUUID})
}
