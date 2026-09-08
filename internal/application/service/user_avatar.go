// User avatar storage (identity-scoped /auth/me/avatar surface). The
// users.avatar column has existed since 000001 and rides every read path
// (login, /auth/me, member lists); these methods are the missing write side.
package service

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/Tencent/WeKnora/internal/logger"
	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
)

// Sentinel errors for the avatar surface. Handlers map these onto distinct
// HTTP statuses so the UI can tell "join a workspace first" apart from
// "storage is broken".
var (
	// ErrAvatarStorageUnavailable means no FileService is wired (partial DI
	// graph); reads and writes both refuse rather than silently no-op.
	ErrAvatarStorageUnavailable = errors.New("avatar storage unavailable")

	// ErrAvatarNoWorkspace means neither the user's home tenant nor the
	// request context resolves a tenant. Resource registration requires an
	// owner tenant, so a tenantless bootstrap admin cannot upload yet.
	ErrAvatarNoWorkspace = errors.New("join a workspace before setting an avatar")

	// ErrUserAvatarNotSet means users.avatar is empty — render the default.
	ErrUserAvatarNotSet = errors.New("no avatar set")
)

// avatarTenantID resolves the tenant that owns the avatar object. Home
// tenant first; tenantless users fall back to the request's active tenant
// (an admin bootstrapping inside their first workspace still works). Zero
// means the catalog would reject the registration — surfaced as
// ErrAvatarNoWorkspace by the caller.
func avatarTenantID(ctx context.Context, user *types.User) uint64 {
	if user.TenantID != 0 {
		return user.TenantID
	}
	if tenantID, ok := types.TenantIDFromContext(ctx); ok {
		return tenantID
	}
	return 0
}

// SetUserAvatar stores the cropped avatar bytes under the user's home
// workspace and persists the returned storage ref in users.avatar. The
// replaced object is deleted best-effort after the column write succeeds —
// an orphan beats showing a broken ref.
func (s *userService) SetUserAvatar(
	ctx context.Context, user *types.User, data []byte, ext string,
) (*types.User, error) {
	if s.fileService == nil {
		return nil, ErrAvatarStorageUnavailable
	}
	tenantID := avatarTenantID(ctx, user)
	if tenantID == 0 {
		return nil, ErrAvatarNoWorkspace
	}

	ref, err := s.fileService.SaveBytes(
		ctx, data, tenantID, "avatar-"+user.ID+ext, false)
	if err != nil {
		return nil, fmt.Errorf("store avatar: %w", err)
	}
	if err := s.userRepo.UpdateUserAvatar(ctx, user.ID, ref); err != nil {
		// Roll the stored object back so a failed column write cannot
		// leave an unreachable object nobody references.
		_ = s.fileService.DeleteFile(ctx, ref)
		return nil, fmt.Errorf("persist avatar ref: %w", err)
	}

	if old := strings.TrimSpace(user.Avatar); old != "" && old != ref {
		if err := s.fileService.DeleteFile(ctx, old); err != nil {
			logger.Warnf(ctx, "[UserAvatar] delete replaced avatar failed: user_id=%s ref=%s err=%v",
				user.ID, old, err)
		}
	}

	updated := *user
	updated.Avatar = ref
	return &updated, nil
}

// DeleteUserAvatar clears users.avatar and removes the stored object
// best-effort. Idempotent: deleting again when no avatar is set succeeds.
func (s *userService) DeleteUserAvatar(ctx context.Context, user *types.User) (*types.User, error) {
	if s.fileService == nil {
		return nil, ErrAvatarStorageUnavailable
	}
	old := strings.TrimSpace(user.Avatar)
	if old == "" {
		return user, nil
	}
	if err := s.userRepo.UpdateUserAvatar(ctx, user.ID, ""); err != nil {
		return nil, fmt.Errorf("clear avatar ref: %w", err)
	}
	if err := s.fileService.DeleteFile(ctx, old); err != nil {
		logger.Warnf(ctx, "[UserAvatar] delete avatar object failed: user_id=%s ref=%s err=%v",
			user.ID, old, err)
	}

	updated := *user
	updated.Avatar = ""
	return &updated, nil
}

// OpenUserAvatar opens the stored avatar bytes for streaming. The MIME type
// comes from the resource catalog registration (derived from the extension
// at save time); image/jpeg is the fallback for pre-catalog refs.
func (s *userService) OpenUserAvatar(
	ctx context.Context, user *types.User,
) (io.ReadCloser, string, error) {
	if s.fileService == nil {
		return nil, "", ErrAvatarStorageUnavailable
	}
	ref := strings.TrimSpace(user.Avatar)
	if ref == "" {
		return nil, "", ErrUserAvatarNotSet
	}

	contentType := "image/jpeg"
	if s.resourceCatalog != nil {
		if _, res, err := s.resourceCatalog.ResolvePath(ctx, ref); err == nil && res != nil && res.MimeType != "" {
			contentType = res.MimeType
		}
	}

	reader, err := s.fileService.GetFile(ctx, ref)
	if err != nil {
		return nil, "", fmt.Errorf("open avatar: %w", err)
	}
	return reader, contentType, nil
}

// Compile-time guard: the avatar methods complete the UserService contract.
var _ interfaces.UserService = (*userService)(nil)
