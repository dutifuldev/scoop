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

// mutate4go-manifest-begin
// {"version":1,"tested_at":"2026-07-23T14:18:58+08:00","module_hash":"4927d80c6f38421d691aeacaa05b315f0d0c1c22d0523a10951c38c8ee339376","functions":[{"id":"func/Server.handlePersonIdentities","name":"Server.handlePersonIdentities","line":16,"end_line":25,"hash":"1bb696231be6b0e9bcab13e1438b8ad491095299cd9f9859a16daa346252fe1c"},{"id":"func/Server.handleArticlePersonIdentities","name":"Server.handleArticlePersonIdentities","line":27,"end_line":38,"hash":"6c4f379f6b77ad289062e27f25fd04ee737801437e8c02bf824b0c16af158411"},{"id":"func/Server.handleAddArticlePersonIdentity","name":"Server.handleAddArticlePersonIdentity","line":40,"end_line":62,"hash":"bd5d6f8f8b8bea9a4c49b649ccb2193df2a809025ab75b1dfca4be1a2ff77ef0"},{"id":"func/Server.handleRemoveArticlePersonIdentity","name":"Server.handleRemoveArticlePersonIdentity","line":64,"end_line":82,"hash":"cbc253cc53462cadafa9d843eeec6dbba9d5fe31df65f5cbf9c547790a464078"}]}
// mutate4go-manifest-end
