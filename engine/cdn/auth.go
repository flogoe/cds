package cdn

import (
	"context"
	"net/http"

	"github.com/dgrijalva/jwt-go"
	"github.com/gorilla/mux"
	"github.com/ovh/cds/engine/authentication"
	"github.com/ovh/cds/engine/cache"
	"github.com/ovh/cds/engine/cdn/item"
	"github.com/ovh/cds/engine/service"
	"github.com/ovh/cds/sdk"
	"github.com/ovh/cds/sdk/telemetry"
)

var (
	keyPermission = cache.Key("cdn", "permission")
)

func (s *Service) jwtMiddleware(ctx context.Context, w http.ResponseWriter, req *http.Request, rc *service.HandlerConfig) (context.Context, error) {
	ctx, end := telemetry.Span(ctx, "router.jwtMiddleware")
	defer end()

	v := authentication.NewVerifier(s.ParsedAPIPublicKey)
	return service.JWTMiddleware(ctx, w, req, rc, v.VerifyJWT)
}

func (s *Service) itemAccessMiddleware(ctx context.Context, w http.ResponseWriter, req *http.Request, rc *service.HandlerConfig) (context.Context, error) {
	ctx, end := telemetry.Span(ctx, "router.itemAccessMiddleware")
	defer end()

	vars := mux.Vars(req)
	itemTypeRaw, ok := vars["type"]
	if !ok {
		return ctx, sdk.WithStack(sdk.ErrUnauthorized)
	}
	itemType := sdk.CDNItemType(itemTypeRaw)
	if err := itemType.Validate(); err != nil {
		return ctx, sdk.NewErrorWithStack(err, sdk.ErrUnauthorized)
	}
	apiRef, ok := vars["apiRef"]
	if !ok {
		return ctx, sdk.WithStack(sdk.ErrUnauthorized)
	}

	return s.itemAccessCheck(ctx, itemType, apiRef)
}

func (s *Service) itemAccessCheck(ctx context.Context, itemType sdk.CDNItemType, apiRef string) (context.Context, error) {
	ctx, end := telemetry.Span(ctx, "router.itemAccessCheck")
	defer end()

	// Check for session based on jwt from context
	jwt, ok := ctx.Value(service.ContextJWT).(*jwt.Token)
	if !ok {
		return ctx, sdk.WithStack(sdk.ErrUnauthorized)
	}
	claims := jwt.Claims.(*sdk.AuthSessionJWTClaims)
	sessionID := claims.StandardClaims.Id

	keyWorkflowPermissionForSession := cache.Key(keyPermission, apiRef, sessionID)

	exists, err := s.Cache.Exist(keyWorkflowPermissionForSession)
	if err != nil {
		return ctx, sdk.NewErrorWithStack(sdk.WrapError(err, "unable to check if permission %s exists", keyWorkflowPermissionForSession), sdk.ErrUnauthorized)
	}
	if exists {
		return ctx, nil
	}

	item, err := item.LoadByAPIRefHashAndType(ctx, s.Mapper, s.mustDBWithCtx(ctx), apiRef, itemType)
	if err != nil {
		return ctx, sdk.NewErrorWithStack(err, sdk.ErrNotFound)
	}

	if err := s.Client.WorkflowLogAccess(ctx, item.APIRef.ProjectKey, item.APIRef.WorkflowName, sessionID); err != nil {
		return ctx, sdk.NewErrorWithStack(err, sdk.ErrNotFound)
	}

	if err := s.Cache.SetWithTTL(keyWorkflowPermissionForSession, true, 3600); err != nil {
		return ctx, sdk.NewErrorWithStack(sdk.WrapError(err, "unable to store permission %s", keyWorkflowPermissionForSession), sdk.ErrUnauthorized)
	}

	return ctx, nil
}
