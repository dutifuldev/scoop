package httpapi

import (
	"errors"
	"strings"

	"github.com/labstack/echo/v4"

	"horse.fit/scoop/internal/db"
)

func articleUUIDFromParam(c echo.Context) (string, error) {
	articleUUID := strings.TrimSpace(c.Param("article_uuid"))
	if articleUUID == "" {
		return "", failValidation(c, map[string]string{"article_uuid": "is required"})
	}
	return articleUUID, nil
}

func actorUserIDFromContext(c echo.Context) (*int64, bool) {
	principal, ok := principalFromContext(c)
	if !ok {
		_ = unauthorizedResponse(c)
		return nil, false
	}
	actorUserID := principal.UserID
	return &actorUserID, true
}

func (s *Server) handleArticleRelationMutationError(
	c echo.Context,
	err error,
	notFound string,
	validationField string,
	relationField string,
	relationValue string,
	logMessage string,
	clientMessage string,
) error {
	if errors.Is(err, db.ErrNoRows) {
		return failNotFound(c, notFound)
	}
	if msg := mutationValidationMessage(err); msg != "" {
		return failValidation(c, map[string]string{validationField: msg})
	}
	s.logger.Error().Err(err).Str("article_uuid", c.Param("article_uuid")).Str(relationField, relationValue).Msg(logMessage)
	return internalError(c, clientMessage)
}
