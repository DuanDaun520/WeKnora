package service

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
	"golang.org/x/crypto/bcrypt"
)

// adminCreateUserRepo records the CreateUser call so tests can verify both
// the persisted user and the exact password bytes handed to bcrypt.
type adminCreateUserRepo struct {
	interfaces.UserRepository
	existingByEmployeeID *types.User
	created              *types.User
}

func (r *adminCreateUserRepo) GetUserByEmployeeID(_ context.Context, _ string) (*types.User, error) {
	if r.existingByEmployeeID != nil {
		return r.existingByEmployeeID, nil
	}
	return nil, nil
}

func (r *adminCreateUserRepo) CreateUser(_ context.Context, user *types.User) error {
	copied := *user
	r.created = &copied
	return nil
}

func newAdminCreateUserService(repo *adminCreateUserRepo) *userService {
	return &userService{userRepo: repo, tenantService: nil, memberService: nil}
}

func TestAdminCreateUserGeneratesPolicyCompliantPasswordWhenEmpty(t *testing.T) {
	repo := &adminCreateUserRepo{}
	svc := newAdminCreateUserService(repo)

	user, generated, err := svc.AdminCreateUser(context.Background(), &types.AdminCreateUserRequest{
		EmployeeID: "10001", Username: "alice",
	})
	if err != nil {
		t.Fatalf("AdminCreateUser: %v", err)
	}
	if generated == "" {
		t.Fatal("expected a generated password")
	}
	if user == nil || repo.created == nil {
		t.Fatalf("user was not persisted: %v", repo.created)
	}

	if err := ValidatePasswordPolicy(generated); err != nil {
		t.Fatalf("generated password violates the policy: %v", err)
	}
	if bcrypt.CompareHashAndPassword([]byte(repo.created.PasswordHash), []byte(generated)) != nil {
		t.Fatal("persisted hash does not match the generated password")
	}
}

func TestAdminCreateUserProvisionsTenantlessForcedRotationAccount(t *testing.T) {
	repo := &adminCreateUserRepo{}
	svc := newAdminCreateUserService(repo)

	email := "alice@example.com"
	_, _, err := svc.AdminCreateUser(context.Background(), &types.AdminCreateUserRequest{
		EmployeeID: "10001", Username: "alice", Email: &email, Password: new("PlainPass9"),
	})
	if err != nil {
		t.Fatalf("AdminCreateUser: %v", err)
	}
	created := repo.created
	if created.EmployeeID != "10001" {
		t.Fatalf("EmployeeID=%q, want 10001", created.EmployeeID)
	}
	if created.TenantID != 0 {
		t.Fatalf("TenantID=%d, want 0 (tenantless until an admin binds workspaces)", created.TenantID)
	}
	if !created.MustChangePassword {
		t.Fatal("MustChangePassword must be true so the initial password is rotated on first login")
	}
	if !created.IsActive {
		t.Fatal("IsActive must be true")
	}
	if created.IsSystemAdmin {
		t.Fatal("provisioned users must not be system admins")
	}
	if created.Email != email {
		t.Fatalf("Email=%q, want the optional contact field preserved", created.Email)
	}
}

func TestAdminCreateUserUsesExplicitPassword(t *testing.T) {
	repo := &adminCreateUserRepo{}
	svc := newAdminCreateUserService(repo)

	user, generated, err := svc.AdminCreateUser(context.Background(), &types.AdminCreateUserRequest{
		EmployeeID: "10001", Username: "alice", Password: new("PlainPass9"),
	})
	if err != nil {
		t.Fatalf("AdminCreateUser: %v", err)
	}
	if generated != "" {
		t.Fatalf("generated password must be empty for a caller-supplied password, got %q", generated)
	}
	if user == nil {
		t.Fatal("user is nil")
	}
	if bcrypt.CompareHashAndPassword([]byte(repo.created.PasswordHash), []byte("PlainPass9")) != nil {
		t.Fatal("persisted hash does not match the explicit password")
	}
}

func TestAdminCreateUserHashesUntrimmedPasswordByteForByte(t *testing.T) {
	// Leading/trailing whitespace is part of the credential.
	repo := &adminCreateUserRepo{}
	svc := newAdminCreateUserService(repo)

	raw := "  PlainPass9  "
	if _, _, err := svc.AdminCreateUser(context.Background(), &types.AdminCreateUserRequest{
		EmployeeID: "10001", Username: "alice", Password: &raw,
	}); err != nil {
		t.Fatalf("AdminCreateUser: %v", err)
	}
	if bcrypt.CompareHashAndPassword([]byte(repo.created.PasswordHash), []byte(raw)) != nil {
		t.Fatal("hash does not match the raw password bytes")
	}
	if bcrypt.CompareHashAndPassword([]byte(repo.created.PasswordHash), []byte(strings.TrimSpace(raw))) == nil {
		t.Fatal("hash matches the trimmed password, the credential was rewritten")
	}
}

func TestAdminCreateUserRejectsPolicyViolatingPassword(t *testing.T) {
	// Only an absent password triggers generation; any provided value is
	// policy-checked.
	repo := &adminCreateUserRepo{}
	svc := newAdminCreateUserService(repo)

	// 新策略只要求 ≥6 位；此处全部为不足 6 位的取值。
	for _, pw := range []string{"12345", "", "   ", "\t\n", "    "} {
		_, generated, err := svc.AdminCreateUser(context.Background(), &types.AdminCreateUserRequest{
			EmployeeID: "10001", Username: "alice", Password: &pw,
		})
		if !errors.Is(err, ErrPasswordPolicy) {
			t.Fatalf("password=%q err=%v, want ErrPasswordPolicy", pw, err)
		}
		if generated != "" {
			t.Fatalf("password=%q generated=%q, want no generated password", pw, generated)
		}
		if repo.created != nil {
			t.Fatalf("password=%q reached persistence", pw)
		}
	}
}

func TestGeneratePolicyCompliantPasswordAlwaysComplies(t *testing.T) {
	// A 24-byte base64url draw yields 32 characters, comfortably above
	// the 6-character minimum; sample several draws to catch regressions.
	for i := range 100 {
		pw, err := generatePolicyCompliantPassword()
		if err != nil {
			t.Fatalf("iteration %d: failed to generate password: %v", i, err)
		}
		if err := ValidatePasswordPolicy(pw); err != nil {
			t.Fatalf("iteration %d: generated password %q violates the policy: %v", i, pw, err)
		}
	}
}

func TestAdminCreateUserRejectsWeakPasswordBeforePersisting(t *testing.T) {
	// Providing the password key with any value subjects it to the
	// policy; the explicit empty string is rejected like any other
	// policy-violating value and never reaches persistence.
	repo := &adminCreateUserRepo{}
	svc := newAdminCreateUserService(repo)

	for _, pw := range []string{"12345", ""} {
		_, _, err := svc.AdminCreateUser(context.Background(), &types.AdminCreateUserRequest{
			EmployeeID: "10001", Username: "alice", Password: &pw,
		})
		if !errors.Is(err, ErrPasswordPolicy) {
			t.Fatalf("password=%q err=%v, want ErrPasswordPolicy", pw, err)
		}
		if repo.created != nil {
			t.Fatalf("password=%q reached persistence", pw)
		}
	}
}

func TestAdminCreateUserDuplicateEmployeeIDReturnsExistingUserWithSentinel(t *testing.T) {
	existing := &types.User{ID: "existing", EmployeeID: "10001", Username: "alice"}
	repo := &adminCreateUserRepo{existingByEmployeeID: existing}
	svc := newAdminCreateUserService(repo)

	user, generated, err := svc.AdminCreateUser(context.Background(), &types.AdminCreateUserRequest{
		EmployeeID: "10001", Username: "alice", Password: new("PlainPass9"),
	})
	if !errors.Is(err, ErrUserEmployeeIDExists) {
		t.Fatalf("err=%v, want ErrUserEmployeeIDExists", err)
	}
	if user == nil || user.ID != existing.ID {
		t.Fatalf("user=%v, want the existing user %q", user, existing.ID)
	}
	if generated != "" {
		t.Fatalf("generated=%q, want no generated password for an existing user", generated)
	}
	if repo.created != nil {
		t.Fatal("existing user was overwritten")
	}
}

func TestAdminCreateUserRejectsMissingIdentity(t *testing.T) {
	repo := &adminCreateUserRepo{}
	svc := newAdminCreateUserService(repo)

	for _, req := range []*types.AdminCreateUserRequest{
		{EmployeeID: "", Username: "alice"},
		{EmployeeID: "10001", Username: ""},
		{EmployeeID: "   ", Username: "alice"},
	} {
		if _, _, err := svc.AdminCreateUser(context.Background(), req); err == nil {
			t.Fatalf("req=%+v: expected an error for a missing identity", req)
		}
		if repo.created != nil {
			t.Fatal("invalid request reached persistence")
		}
	}
}

func TestAdminCreateUserAllowsDuplicateDisplayIdentity(t *testing.T) {
	// Real names collide; username/email are display/contact fields. Two
	// accounts sharing them but with distinct employee IDs must both
	// persist — the service performs no uniqueness lookup on them.
	repo := &adminCreateUserRepo{}
	svc := newAdminCreateUserService(repo)

	email := "alice@example.com"
	for _, id := range []string{"10001", "10002"} {
		if _, _, err := svc.AdminCreateUser(context.Background(), &types.AdminCreateUserRequest{
			EmployeeID: id, Username: "王芳", Email: &email, Password: new("PlainPass9"),
		}); err != nil {
			t.Fatalf("employee %s: AdminCreateUser: %v", id, err)
		}
	}
	if repo.created == nil || repo.created.EmployeeID != "10002" {
		t.Fatalf("created=%v, want the second account persisted", repo.created)
	}
}
