package handlers

import (
	"encoding/base64"
	"errors"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/synctv-org/synctv/internal/conf"
	"github.com/synctv-org/synctv/internal/db"
	"github.com/synctv-org/synctv/internal/guardian"
	"github.com/synctv-org/synctv/internal/op"
	"github.com/synctv-org/synctv/server/middlewares"
	"github.com/synctv-org/synctv/server/model"
)

func RootGetManagedUserPassword(ctx *gin.Context) {
	root := middlewares.GetUserEntry(ctx).Value()
	log := middlewares.GetLogger(ctx)
	if !managedPasswordRequestIsSecure(ctx.Request) {
		ctx.AbortWithStatusJSON(
			http.StatusUpgradeRequired,
			model.NewAPIErrorStringResp(
				"managed passwords can only be viewed over HTTPS",
			),
		)
		return
	}
	ctx.Request.Body = http.MaxBytesReader(ctx.Writer, ctx.Request.Body, 4096)

	var req model.ManagedUserPasswordReq
	if err := model.Decode(ctx, &req); err != nil {
		ctx.AbortWithStatusJSON(http.StatusBadRequest, model.NewAPIErrorResp(err))
		return
	}

	credential, err := db.GetManagedUserCredential(req.ID)
	if err != nil {
		switch {
		case errors.Is(err, guardian.ErrKeyNotConfigured),
			errors.Is(err, guardian.ErrInvalidKey):
			ctx.AbortWithStatusJSON(
				http.StatusServiceUnavailable,
				model.NewAPIErrorStringResp("guardian credential key is not configured"),
			)
		case errors.Is(err, db.ErrManagedCredentialNotAvailable):
			ctx.AbortWithStatusJSON(
				http.StatusConflict,
				model.NewAPIErrorStringResp(
					"managed password is unavailable; let the user change it or reset it first",
				),
			)
		case errors.Is(err, db.ErrUserNotManaged):
			ctx.AbortWithStatusJSON(
				http.StatusBadRequest,
				model.NewAPIErrorStringResp("this account is not a managed local user"),
			)
		default:
			log.Errorf("get managed password for user %s: %v", req.ID, err)
			ctx.AbortWithStatusJSON(http.StatusInternalServerError, model.NewAPIErrorResp(err))
		}
		return
	}

	log.Infof("root %s requested managed-password envelope for user %s", root.ID, req.ID)
	ctx.Header("Cache-Control", "no-store")
	ctx.Header("Pragma", "no-cache")
	ctx.JSON(http.StatusOK, model.NewAPIDataResp(&model.ManagedUserPasswordResp{
		Version:   credential.FormatVersion,
		Algorithm: guardian.EnvelopeAlgorithm,
		Envelope:  base64.StdEncoding.EncodeToString(credential.Ciphertext),
		UpdatedAt: credential.UpdatedAt.UnixMilli(),
	}))
}

func managedPasswordRequestIsSecure(request *http.Request) bool {
	if !conf.Conf.Security.GuardianRequireHTTPS || request.TLS != nil {
		return true
	}
	if !conf.Conf.Security.GuardianTrustForwardedProto {
		return false
	}
	forwardedProto := strings.TrimSpace(strings.Split(request.Header.Get("X-Forwarded-Proto"), ",")[0])
	return strings.EqualFold(forwardedProto, "https")
}

func RootAddAdmin(ctx *gin.Context) {
	user := middlewares.GetUserEntry(ctx).Value()
	log := middlewares.GetLogger(ctx)

	req := model.IDReq{}
	if err := model.Decode(ctx, &req); err != nil {
		log.Errorf("failed to decode request: %v", err)
		ctx.AbortWithStatusJSON(http.StatusBadRequest, model.NewAPIErrorResp(err))
		return
	}

	if req.ID == user.ID {
		log.Errorf("cannot add yourself")
		ctx.AbortWithStatusJSON(
			http.StatusBadRequest,
			model.NewAPIErrorStringResp("cannot add yourself"),
		)

		return
	}

	u, err := op.LoadOrInitUserByID(req.ID)
	if err != nil {
		log.Errorf("failed to load user: %v", err)
		ctx.AbortWithStatusJSON(
			http.StatusInternalServerError,
			model.NewAPIErrorStringResp("user not found"),
		)

		return
	}

	if u.Value().IsAdmin() {
		log.Errorf("user is already admin")
		ctx.AbortWithStatusJSON(
			http.StatusBadRequest,
			model.NewAPIErrorStringResp("user is already admin"),
		)

		return
	}

	if err := u.Value().SetAdminRole(); err != nil {
		log.Errorf("failed to set role: %v", err)
		ctx.AbortWithStatusJSON(http.StatusInternalServerError, model.NewAPIErrorResp(err))
		return
	}

	ctx.Status(http.StatusNoContent)
}

func RootDeleteAdmin(ctx *gin.Context) {
	user := middlewares.GetUserEntry(ctx)
	log := middlewares.GetLogger(ctx)

	req := model.AdminUserPasswordReq{}
	if err := model.Decode(ctx, &req); err != nil {
		log.Errorf("failed to decode request: %v", err)
		ctx.AbortWithStatusJSON(http.StatusBadRequest, model.NewAPIErrorResp(err))
		return
	}

	if req.ID == user.Value().ID {
		log.Errorf("cannot remove yourself")
		ctx.AbortWithStatusJSON(
			http.StatusBadRequest,
			model.NewAPIErrorStringResp("cannot remove yourself"),
		)

		return
	}

	u, err := op.LoadOrInitUserByID(req.ID)
	if err != nil {
		log.Errorf("failed to load user: %v", err)
		ctx.AbortWithStatusJSON(
			http.StatusInternalServerError,
			model.NewAPIErrorStringResp("user not found"),
		)

		return
	}

	if u.Value().IsRoot() {
		log.Errorf("cannot remove root")
		ctx.AbortWithStatusJSON(
			http.StatusBadRequest,
			model.NewAPIErrorStringResp("cannot remove root"),
		)

		return
	}

	if err := u.Value().DemoteToManagedUser(req.Password); err != nil {
		log.Errorf("failed to set role: %v", err)
		ctx.AbortWithStatusJSON(http.StatusInternalServerError, model.NewAPIErrorResp(err))
		return
	}

	ctx.Status(http.StatusNoContent)
}
