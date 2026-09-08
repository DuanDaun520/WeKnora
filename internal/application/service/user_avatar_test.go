package service

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
)

// fakeAvatarUserRepo embeds the interface so only UpdateUserAvatar needs a
// body; the avatar surface touches nothing else on the repo.
type fakeAvatarUserRepo struct {
	interfaces.UserRepository
	mu        sync.Mutex
	avatars   map[string]string
	updateErr error
}

func (f *fakeAvatarUserRepo) UpdateUserAvatar(_ context.Context, userID, avatar string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.updateErr != nil {
		return f.updateErr
	}
	if f.avatars == nil {
		f.avatars = map[string]string{}
	}
	f.avatars[userID] = avatar
	return nil
}

func (f *fakeAvatarUserRepo) avatarOf(userID string) string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.avatars[userID]
}

// fakeAvatarFileService records saves/deletes and serves back what it
// stored, so the rollback and replace paths can be asserted on real refs.
type fakeAvatarFileService struct {
	interfaces.FileService
	mu      sync.Mutex
	stored  map[string][]byte
	deleted []string
	saveErr error
}

func (f *fakeAvatarFileService) SaveBytes(
	_ context.Context, data []byte, tenantID uint64, fileName string, _ bool,
) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.saveErr != nil {
		return "", f.saveErr
	}
	if f.stored == nil {
		f.stored = map[string][]byte{}
	}
	ref := fmt.Sprintf("resource://%d/%s", tenantID, fileName)
	f.stored[ref] = append([]byte(nil), data...)
	return ref, nil
}

func (f *fakeAvatarFileService) GetFile(_ context.Context, ref string) (io.ReadCloser, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if data, ok := f.stored[ref]; ok {
		return io.NopCloser(strings.NewReader(string(data))), nil
	}
	return nil, errors.New("avatar object not found")
}

func (f *fakeAvatarFileService) DeleteFile(_ context.Context, ref string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	delete(f.stored, ref)
	f.deleted = append(f.deleted, ref)
	return nil
}

func (f *fakeAvatarFileService) deletedRefs() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.deleted...)
}

// fakeAvatarCatalog only answers ResolvePath with a stored MIME type.
type fakeAvatarCatalog struct {
	interfaces.ResourceCatalog
	mime string
}

func (f *fakeAvatarCatalog) ResolvePath(
	_ context.Context, _ string,
) (string, *types.StoredResource, error) {
	return "", &types.StoredResource{MimeType: f.mime}, nil
}

// newAvatarUserService takes interface-typed deps so a literal nil stays a
// nil interface (a nil *fakeAvatarFileService would box non-nil and panic).
func newAvatarUserService(
	repo interfaces.UserRepository, files interfaces.FileService, catalog interfaces.ResourceCatalog,
) *userService {
	return &userService{
		userRepo:        repo,
		fileService:     files,
		resourceCatalog: catalog,
	}
}

func TestSetUserAvatarPersistsRefAndReturnsCopy(t *testing.T) {
	repo := &fakeAvatarUserRepo{}
	files := &fakeAvatarFileService{}
	svc := newAvatarUserService(repo, files, nil)
	user := &types.User{ID: "u-1", TenantID: 7}
	ctx := context.Background()

	updated, err := svc.SetUserAvatar(ctx, user, []byte("png-bytes"), ".png")
	require.NoError(t, err)

	ref := repo.avatarOf("u-1")
	require.NotEmpty(t, ref)
	require.Contains(t, ref, "avatar-u-1.png")
	require.Equal(t, ref, updated.Avatar)
	require.Equal(t, uint64(7), parseTenantFromRef(t, ref), "stored under the home tenant")
	require.Empty(t, user.Avatar, "the context-shared user must not be mutated")
}

func parseTenantFromRef(t *testing.T, ref string) uint64 {
	t.Helper()
	var tenantID uint64
	_, err := fmt.Sscanf(ref, "resource://%d/", &tenantID)
	require.NoError(t, err)
	return tenantID
}

func TestSetUserAvatarReplacesAndDeletesOldRef(t *testing.T) {
	repo := &fakeAvatarUserRepo{avatars: map[string]string{"u-1": "resource://7/old.png"}}
	files := &fakeAvatarFileService{stored: map[string][]byte{
		"resource://7/old.png": []byte("old"),
	}}
	svc := newAvatarUserService(repo, files, nil)
	user := &types.User{ID: "u-1", TenantID: 7, Avatar: "resource://7/old.png"}

	updated, err := svc.SetUserAvatar(context.Background(), user, []byte("new"), ".png")
	require.NoError(t, err)

	require.Equal(t, updated.Avatar, repo.avatarOf("u-1"))
	require.Contains(t, files.deletedRefs(), "resource://7/old.png")
	require.NotContains(t, files.deletedRefs(), updated.Avatar)
}

func TestSetUserAvatarRollsBackObjectOnColumnFailure(t *testing.T) {
	repo := &fakeAvatarUserRepo{updateErr: errors.New("db down")}
	files := &fakeAvatarFileService{}
	svc := newAvatarUserService(repo, files, nil)
	user := &types.User{ID: "u-1", TenantID: 7}

	_, err := svc.SetUserAvatar(context.Background(), user, []byte("png"), ".png")
	require.Error(t, err)

	require.Len(t, files.deletedRefs(), 1, "the stored object must be rolled back")
}

func TestSetUserAvatarTenantlessResolution(t *testing.T) {
	repo := &fakeAvatarUserRepo{}
	files := &fakeAvatarFileService{}
	svc := newAvatarUserService(repo, files, nil)

	// No home tenant and no context tenant: the catalog would reject the
	// registration, so the service refuses with the sentinel the UI maps.
	_, err := svc.SetUserAvatar(context.Background(),
		&types.User{ID: "u-0", TenantID: 0}, []byte("png"), ".png")
	require.ErrorIs(t, err, ErrAvatarNoWorkspace)

	// An active tenant in the request context (an admin bootstrapping
	// inside their first workspace) is enough to own the object.
	ctx := context.WithValue(context.Background(), types.TenantIDContextKey, uint64(9))
	updated, err := svc.SetUserAvatar(ctx, &types.User{ID: "u-0", TenantID: 0}, []byte("png"), ".png")
	require.NoError(t, err)
	require.Equal(t, uint64(9), parseTenantFromRef(t, updated.Avatar))
}

func TestAvatarSurfaceWithoutFileServiceRefuses(t *testing.T) {
	svc := newAvatarUserService(&fakeAvatarUserRepo{}, nil, nil)
	user := &types.User{ID: "u-1", TenantID: 7, Avatar: "resource://7/a.png"}

	_, err := svc.SetUserAvatar(context.Background(), user, []byte("png"), ".png")
	require.ErrorIs(t, err, ErrAvatarStorageUnavailable)
	_, err = svc.DeleteUserAvatar(context.Background(), user)
	require.ErrorIs(t, err, ErrAvatarStorageUnavailable)
	_, _, err = svc.OpenUserAvatar(context.Background(), user)
	require.ErrorIs(t, err, ErrAvatarStorageUnavailable)
}

func TestDeleteUserAvatarIdempotentAndClearing(t *testing.T) {
	repo := &fakeAvatarUserRepo{avatars: map[string]string{"u-1": "resource://7/a.png"}}
	files := &fakeAvatarFileService{stored: map[string][]byte{
		"resource://7/a.png": []byte("png"),
	}}
	svc := newAvatarUserService(repo, files, nil)

	// No avatar set: succeeds without touching storage.
	untouched, err := svc.DeleteUserAvatar(context.Background(), &types.User{ID: "u-2", TenantID: 7})
	require.NoError(t, err)
	require.Empty(t, untouched.Avatar)
	require.Empty(t, files.deletedRefs())

	// With an avatar: column cleared, object deleted.
	updated, err := svc.DeleteUserAvatar(context.Background(),
		&types.User{ID: "u-1", TenantID: 7, Avatar: "resource://7/a.png"})
	require.NoError(t, err)
	require.Empty(t, updated.Avatar)
	require.Empty(t, repo.avatarOf("u-1"))
	require.Contains(t, files.deletedRefs(), "resource://7/a.png")
}

func TestOpenUserAvatarMimeAndSentinels(t *testing.T) {
	files := &fakeAvatarFileService{stored: map[string][]byte{
		"resource://7/a.png": []byte("png"),
	}}

	// Empty ref is the "render the default" sentinel.
	svc := newAvatarUserService(&fakeAvatarUserRepo{}, files, nil)
	_, _, err := svc.OpenUserAvatar(context.Background(), &types.User{ID: "u-1", TenantID: 7})
	require.ErrorIs(t, err, ErrUserAvatarNotSet)

	// Catalog MIME wins over the fallback.
	withCatalog := newAvatarUserService(&fakeAvatarUserRepo{}, files,
		&fakeAvatarCatalog{mime: "image/webp"})
	reader, contentType, err := withCatalog.OpenUserAvatar(context.Background(),
		&types.User{ID: "u-1", TenantID: 7, Avatar: "resource://7/a.png"})
	require.NoError(t, err)
	defer reader.Close()
	require.Equal(t, "image/webp", contentType)

	// Without a catalog the jpeg fallback applies.
	noCatalog := newAvatarUserService(&fakeAvatarUserRepo{}, files, nil)
	_, contentType, err = noCatalog.OpenUserAvatar(context.Background(),
		&types.User{ID: "u-1", TenantID: 7, Avatar: "resource://7/a.png"})
	require.NoError(t, err)
	require.Equal(t, "image/jpeg", contentType)
}
