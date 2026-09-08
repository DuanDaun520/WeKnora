// Identity-scoped avatar endpoints (POST/GET/DELETE /auth/me/avatar). The
// avatar bytes live in object storage under the user's home workspace, but
// the serving route is deliberately tenant-independent: the /files proxy
// rejects a home-tenant resource:// ref when the caller has switched into a
// peer tenant, and a user's own avatar must follow them across workspaces.
package handler

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	stderrors "errors"
	"io"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/Tencent/WeKnora/internal/application/service"
	apperrors "github.com/Tencent/WeKnora/internal/errors"
	"github.com/Tencent/WeKnora/internal/logger"
	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
)

// userAvatarService is the identity-scoped slice of the user service. Not
// one method takes an identity argument: every call derives the caller from
// the context or receives the freshly resolved context user, so a request
// can never select whose avatar is touched.
type userAvatarService interface {
	GetCurrentUser(ctx context.Context) (*types.User, error)
	SetUserAvatar(ctx context.Context, user *types.User, data []byte, ext string) (*types.User, error)
	DeleteUserAvatar(ctx context.Context, user *types.User) (*types.User, error)
	OpenUserAvatar(ctx context.Context, user *types.User) (io.ReadCloser, string, error)
}

// UserAvatarHandler serves the /auth/me/avatar endpoints.
type UserAvatarHandler struct {
	svc userAvatarService
}

// NewUserAvatarHandler takes the full UserService (the type the container
// provides) but stores it under the narrow interface, so the handler stays
// testable with a tiny fake and documents exactly what it may call.
func NewUserAvatarHandler(svc interfaces.UserService) *UserAvatarHandler {
	return &UserAvatarHandler{svc: svc}
}

// maxAvatarUploadBytes caps the uploaded avatar. The client-side cropper
// exports a 512×512 JPEG (~100-300KB); 2MiB tolerates PNG originals too.
const maxAvatarUploadBytes = 2 << 20

// avatarExtension maps a sniffed image content type onto the storage-file
// extension (which later drives the catalog's MIME registration).
func avatarExtension(contentType string) (string, bool) {
	switch contentType {
	case "image/png":
		return ".png", true
	case "image/jpeg":
		return ".jpg", true
	case "image/webp":
		return ".webp", true
	default:
		return "", false
	}
}

// avatarETag derives a strong ETag from the stored ref. Every upload stores
// a fresh object (new unix-nano suffix), so the ETag changes exactly when
// the avatar does.
func avatarETag(ref string) string {
	sum := sha256.Sum256([]byte(ref))
	return `"` + hex.EncodeToString(sum[:8]) + `"`
}

// respondAvatarError renders a service refusal. The three sentinels carry
// meaning the UI acts on; anything else is an internal failure.
func respondAvatarError(c *gin.Context, err error) {
	switch {
	case stderrors.Is(err, service.ErrAvatarNoWorkspace):
		c.Error(apperrors.NewBadRequestError(err.Error()))
	case stderrors.Is(err, service.ErrUserAvatarNotSet):
		c.Error(apperrors.NewNotFoundError(err.Error()))
	default:
		c.Error(err)
	}
}

// currentUserOrRespond resolves the caller from the context, writing the
// 401 itself when the context has no user.
func (h *UserAvatarHandler) currentUserOrRespond(c *gin.Context) (*types.User, bool) {
	user, err := h.svc.GetCurrentUser(c.Request.Context())
	if err != nil || user == nil {
		c.Error(apperrors.NewUnauthorizedError("Failed to get user information").WithDetails(errString(err)))
		return nil, false
	}
	return user, true
}

func errString(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

// Upload godoc
// @Summary      Upload my avatar
// @Description  Stores a cropped avatar image (png/jpeg/webp, max 2MiB) for the calling user and returns the updated profile.
// @Tags         Auth
// @Accept       multipart/form-data
// @Produce      json
// @Param        file  formData  file  true  "Avatar image"
// @Success      200  {object}  map[string]interface{}
// @Failure      400  {object}  map[string]interface{}
// @Failure      401  {object}  map[string]interface{}
// @Security     Bearer
// @Router       /auth/me/avatar [post]
func (h *UserAvatarHandler) Upload(c *gin.Context) {
	ctx := c.Request.Context()

	limitUploadBody(c, maxAvatarUploadBytes)
	fileHeader, err := c.FormFile("file")
	if err != nil {
		if isRequestBodyTooLarge(err) {
			c.Error(apperrors.NewBadRequestError("avatar file too large (max 2MB)"))
			return
		}
		c.Error(apperrors.NewValidationError("avatar file is required"))
		return
	}

	f, err := fileHeader.Open()
	if err != nil {
		c.Error(apperrors.NewValidationError("avatar file cannot be read").WithDetails(err.Error()))
		return
	}
	defer f.Close()
	data, err := io.ReadAll(io.LimitReader(f, maxAvatarUploadBytes+1))
	if err != nil {
		c.Error(apperrors.NewValidationError("avatar file cannot be read").WithDetails(err.Error()))
		return
	}
	if len(data) == 0 {
		c.Error(apperrors.NewValidationError("avatar file is empty"))
		return
	}
	if len(data) > maxAvatarUploadBytes {
		c.Error(apperrors.NewBadRequestError("avatar file too large (max 2MB)"))
		return
	}

	// Sniff the actual bytes — a renamed .txt must not ride the part name.
	ext, ok := avatarExtension(http.DetectContentType(data))
	if !ok {
		c.Error(apperrors.NewValidationError("unsupported avatar image type; expected png, jpeg or webp"))
		return
	}

	user, okUser := h.currentUserOrRespond(c)
	if !okUser {
		return
	}
	updated, err := h.svc.SetUserAvatar(ctx, user, data, ext)
	if err != nil {
		respondAvatarError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": gin.H{"user": updated.ToUserInfo()}})
}

// Download godoc
// @Summary      Download my avatar
// @Description  Streams the calling user's stored avatar bytes. Versioned by ETag; the frontend also appends ?v=<ref> so a new upload is a new URL.
// @Tags         Auth
// @Produce      image/*
// @Success      200  {file}  binary
// @Failure      401  {object}  map[string]interface{}
// @Failure      404  {object}  map[string]interface{}
// @Security     Bearer
// @Router       /auth/me/avatar [get]
func (h *UserAvatarHandler) Download(c *gin.Context) {
	user, ok := h.currentUserOrRespond(c)
	if !ok {
		return
	}

	etag := avatarETag(user.Avatar)
	// Cache headers land before both the 304 short-circuit and the 200 so
	// a revalidation response carries the same policy as a full one.
	c.Header("Cache-Control", "private, max-age=86400")
	c.Header("ETag", etag)
	if c.Request.Header.Get("If-None-Match") == etag {
		c.Status(http.StatusNotModified)
		return
	}

	reader, contentType, err := h.svc.OpenUserAvatar(c.Request.Context(), user)
	if err != nil {
		respondAvatarError(c, err)
		return
	}
	defer reader.Close()

	c.Header("Content-Type", contentType)
	c.Header("X-Content-Type-Options", "nosniff")
	c.Header("Content-Disposition", "inline")
	c.Status(http.StatusOK)
	if _, err := io.Copy(c.Writer, reader); err != nil {
		// Headers are already sent; the copy failure only breaks the stream.
		logger.Warnf(c.Request.Context(), "[UserAvatar] write response failed: %v", err)
	}
}

// Delete godoc
// @Summary      Remove my avatar
// @Description  Clears the calling user's avatar and deletes the stored object best-effort. Idempotent.
// @Tags         Auth
// @Produce      json
// @Success      200  {object}  map[string]interface{}
// @Failure      401  {object}  map[string]interface{}
// @Security     Bearer
// @Router       /auth/me/avatar [delete]
func (h *UserAvatarHandler) Delete(c *gin.Context) {
	user, ok := h.currentUserOrRespond(c)
	if !ok {
		return
	}
	updated, err := h.svc.DeleteUserAvatar(c.Request.Context(), user)
	if err != nil {
		respondAvatarError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": gin.H{"user": updated.ToUserInfo()}})
}
