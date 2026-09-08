package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/Tencent/WeKnora/internal/application/service"
	"github.com/Tencent/WeKnora/internal/middleware"
	"github.com/Tencent/WeKnora/internal/types"
)

type fakeUserAvatarService struct {
	user *types.User

	setData []byte
	setExt  string
	deleted bool
	opened  bool

	setErr error
	delErr error
}

func (f *fakeUserAvatarService) GetCurrentUser(context.Context) (*types.User, error) {
	if f.user == nil {
		return nil, errors.New("no user in context")
	}
	return f.user, nil
}

func (f *fakeUserAvatarService) SetUserAvatar(
	_ context.Context, user *types.User, data []byte, ext string,
) (*types.User, error) {
	if f.setErr != nil {
		return nil, f.setErr
	}
	f.setData, f.setExt = data, ext
	updated := *user
	updated.Avatar = "resource://7/avatar-" + user.ID + ext
	return &updated, nil
}

func (f *fakeUserAvatarService) DeleteUserAvatar(
	_ context.Context, user *types.User,
) (*types.User, error) {
	if f.delErr != nil {
		return nil, f.delErr
	}
	f.deleted = true
	updated := *user
	updated.Avatar = ""
	return &updated, nil
}

func (f *fakeUserAvatarService) OpenUserAvatar(
	_ context.Context, user *types.User,
) (io.ReadCloser, string, error) {
	// Mirrors the real service: an empty ref is the "render the default"
	// sentinel, surfaced as ErrUserAvatarNotSet → 404.
	if strings.TrimSpace(user.Avatar) == "" {
		return nil, "", service.ErrUserAvatarNotSet
	}
	f.opened = true
	return io.NopCloser(strings.NewReader("avatar-bytes")), "image/png", nil
}

func newUserAvatarRouter(svc userAvatarService) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(middleware.ErrorHandler())
	h := &UserAvatarHandler{svc: svc}
	r.POST("/auth/me/avatar", h.Upload)
	r.GET("/auth/me/avatar", h.Download)
	r.DELETE("/auth/me/avatar", h.Delete)
	return r
}

// pngBody builds a multipart body carrying the given bytes under field
// "file" with the given part filename.
func avatarMultipartBody(t *testing.T, fileName string, data []byte) (*bytes.Buffer, string) {
	t.Helper()
	var body bytes.Buffer
	w := multipart.NewWriter(&body)
	part, err := w.CreateFormFile("file", fileName)
	require.NoError(t, err)
	_, err = part.Write(data)
	require.NoError(t, err)
	require.NoError(t, w.Close())
	return &body, w.FormDataContentType()
}

var pngMagic = append([]byte{0x89, 'P', 'N', 'G', 0x0D, 0x0A, 0x1A, 0x0A},
	bytes.Repeat([]byte("image-data"), 10)...)

func avatarUser(avatar string) *types.User {
	return &types.User{ID: "u-1", Username: "alice", TenantID: 7, Avatar: avatar}
}

func TestUserAvatarUploadStoresSniffedPNG(t *testing.T) {
	svc := &fakeUserAvatarService{user: avatarUser("")}
	router := newUserAvatarRouter(svc)

	body, contentType := avatarMultipartBody(t, "avatar.txt", pngMagic)
	req := httptest.NewRequest(http.MethodPost, "/auth/me/avatar", body)
	req.Header.Set("Content-Type", contentType)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	require.Equal(t, pngMagic, svc.setData, "the sniffed bytes, not the part name, drive the type")
	require.Equal(t, ".png", svc.setExt)

	var resp struct {
		Success bool `json:"success"`
		Data    struct {
			User types.UserInfo `json:"user"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	require.True(t, resp.Success)
	require.NotEmpty(t, resp.Data.User.Avatar)
}

func TestUserAvatarUploadRejectsNonImage(t *testing.T) {
	svc := &fakeUserAvatarService{user: avatarUser("")}
	router := newUserAvatarRouter(svc)

	body, contentType := avatarMultipartBody(t, "avatar.png", []byte("definitely not an image"))
	req := httptest.NewRequest(http.MethodPost, "/auth/me/avatar", body)
	req.Header.Set("Content-Type", contentType)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusBadRequest, rec.Code)
	require.Contains(t, rec.Body.String(), "unsupported avatar image type")
	require.Nil(t, svc.setData)
}

func TestUserAvatarUploadRejectsOversize(t *testing.T) {
	svc := &fakeUserAvatarService{user: avatarUser("")}
	router := newUserAvatarRouter(svc)

	huge := append(append([]byte{}, pngMagic...),
		bytes.Repeat([]byte("x"), maxAvatarUploadBytes)...)
	body, contentType := avatarMultipartBody(t, "avatar.png", huge)
	req := httptest.NewRequest(http.MethodPost, "/auth/me/avatar", body)
	req.Header.Set("Content-Type", contentType)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusBadRequest, rec.Code)
	require.Contains(t, rec.Body.String(), "too large")
	require.Nil(t, svc.setData)
}

func TestUserAvatarUploadRequiresFileField(t *testing.T) {
	svc := &fakeUserAvatarService{user: avatarUser("")}
	router := newUserAvatarRouter(svc)

	var body bytes.Buffer
	w := multipart.NewWriter(&body)
	require.NoError(t, w.Close())
	req := httptest.NewRequest(http.MethodPost, "/auth/me/avatar", &body)
	req.Header.Set("Content-Type", w.FormDataContentType())
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusBadRequest, rec.Code)
	require.Contains(t, rec.Body.String(), "avatar file is required")
}

func TestUserAvatarUploadMapsNoWorkspaceSentinel(t *testing.T) {
	svc := &fakeUserAvatarService{user: avatarUser(""), setErr: service.ErrAvatarNoWorkspace}
	router := newUserAvatarRouter(svc)

	body, contentType := avatarMultipartBody(t, "avatar.png", pngMagic)
	req := httptest.NewRequest(http.MethodPost, "/auth/me/avatar", body)
	req.Header.Set("Content-Type", contentType)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusBadRequest, rec.Code)
	require.Contains(t, rec.Body.String(), "workspace")
}

func TestUserAvatarDownloadStreamsAndRevalidates(t *testing.T) {
	svc := &fakeUserAvatarService{user: avatarUser("resource://7/avatar-u-1.png")}
	router := newUserAvatarRouter(svc)

	req := httptest.NewRequest(http.MethodGet, "/auth/me/avatar", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	require.Equal(t, "image/png", rec.Header().Get("Content-Type"))
	require.Equal(t, "nosniff", rec.Header().Get("X-Content-Type-Options"))
	require.Contains(t, rec.Header().Get("Cache-Control"), "private")
	require.Equal(t, "avatar-bytes", rec.Body.String())

	etag := rec.Header().Get("ETag")
	require.NotEmpty(t, etag, "the ETag rides the full response")

	req = httptest.NewRequest(http.MethodGet, "/auth/me/avatar", nil)
	req.Header.Set("If-None-Match", etag)
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	require.Equal(t, http.StatusNotModified, rec.Code)
	require.Empty(t, rec.Body.String())
}

func TestUserAvatarDownloadWithoutAvatarIs404(t *testing.T) {
	svc := &fakeUserAvatarService{user: avatarUser("")}
	router := newUserAvatarRouter(svc)

	req := httptest.NewRequest(http.MethodGet, "/auth/me/avatar", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusNotFound, rec.Code)
	require.False(t, svc.opened)
}

func TestUserAvatarDeleteClears(t *testing.T) {
	svc := &fakeUserAvatarService{user: avatarUser("resource://7/avatar-u-1.png")}
	router := newUserAvatarRouter(svc)

	req := httptest.NewRequest(http.MethodDelete, "/auth/me/avatar", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	require.True(t, svc.deleted)

	var resp struct {
		Success bool `json:"success"`
		Data    struct {
			User types.UserInfo `json:"user"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	require.Empty(t, resp.Data.User.Avatar)
}

func TestUserAvatarRoutesRequireIdentity(t *testing.T) {
	svc := &fakeUserAvatarService{user: nil}
	router := newUserAvatarRouter(svc)

	for _, method := range []string{http.MethodPost, http.MethodGet, http.MethodDelete} {
		var body *bytes.Buffer
		contentType := ""
		if method == http.MethodPost {
			body, contentType = avatarMultipartBody(t, "avatar.png", pngMagic)
		} else {
			body = &bytes.Buffer{}
		}
		req := httptest.NewRequest(method, "/auth/me/avatar", body)
		if contentType != "" {
			req.Header.Set("Content-Type", contentType)
		}
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)
		require.Equal(t, http.StatusUnauthorized, rec.Code, method)
	}
}
