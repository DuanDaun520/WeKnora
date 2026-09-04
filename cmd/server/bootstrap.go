// Bootstrap-time hooks that run after the DI container is built but
// before the HTTP server starts listening. These are deliberately
// best-effort: any failure here only warns and does NOT abort startup.
// The reasoning is that an operator running with a misconfigured env
// var should still be able to bring the server up (and fix the issue
// from the running instance) rather than have a typo brick the deploy.
package main

import (
	"context"
	"os"
	"strings"

	"go.uber.org/dig"

	"github.com/Tencent/WeKnora/internal/logger"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
)

const (
	// bootstrapAdminEmployeeIDEnv names the employee ID of the default
	// admin account created when a deployment boots with no system
	// administrators. Defaults to defaultBootstrapAdminEmployeeID.
	bootstrapAdminEmployeeIDEnv = "WEKNORA_BOOTSTRAP_ADMIN_EMPLOYEE_ID"
	// bootstrapAdminPasswordEnv optionally fixes the default admin's
	// initial password. When unset a random policy-compliant password is
	// generated and printed to the log exactly once; either way the
	// account starts with MustChangePassword=true.
	bootstrapAdminPasswordEnv = "WEKNORA_BOOTSTRAP_ADMIN_PASSWORD"
	// defaultBootstrapAdminEmployeeID is the employee ID used for the
	// bootstrap account when the env var above is not set.
	defaultBootstrapAdminEmployeeID = "admin"
)

// runStartupBootstrap consults the env and applies any one-shot
// bootstrap actions. Currently: legacy api-key hash backfill and the
// default-admin guarantee; future bootstrap steps (default model
// seeding, etc.) can be added here as additional dig.Invoke calls.
func runStartupBootstrap(c *dig.Container) {
	ctx := context.Background()

	// Legacy hash repair for migration 000065 placeholder rows. Invoked each
	// startup but short-circuits with a cheap EXISTS once every row is
	// backfilled (no api_key decryption on the steady-state path).
	if err := c.Invoke(func(apiKeySvc interfaces.TenantAPIKeyService) {
		if n, err := apiKeySvc.BackfillMissingKeyHashes(ctx); err != nil {
			logger.Warnf(ctx, "[bootstrap] tenant api key hash backfill failed: %v", err)
		} else if n > 0 {
			logger.Infof(ctx, "[bootstrap] backfilled %d legacy tenant api key hash(es)", n)
		}
	}); err != nil {
		logger.Warnf(ctx, "[bootstrap] failed to resolve TenantAPIKeyService: %v", err)
	}

	// dig.Invoke resolves UserService from the container; if user
	// service registration is broken we want to know loudly, but still
	// not abort startup — bootstrap is best-effort.
	if err := c.Invoke(func(userSvc interfaces.UserService) {
		bootstrapDefaultAdmin(ctx, userSvc)
	}); err != nil {
		logger.Warnf(ctx, "[bootstrap] failed to resolve UserService: %v", err)
	}
}

// bootstrapDefaultAdmin guarantees the deployment has a system
// administrator: with none present it creates the default admin account
// (tenantless, forced password change on first login). Safe to leave the
// env vars set permanently — once any sysadmin exists this is a no-op, so
// a UI revoke cannot be silently undone by a restart.
func bootstrapDefaultAdmin(ctx context.Context, userSvc interfaces.UserService) {
	employeeID := strings.TrimSpace(os.Getenv(bootstrapAdminEmployeeIDEnv))
	if employeeID == "" {
		employeeID = defaultBootstrapAdminEmployeeID
	}
	// Deliberately NOT trimmed: passwords may legitimately begin or end
	// with whitespace under the policy.
	password := os.Getenv(bootstrapAdminPasswordEnv)

	changed, generatedPassword, err := userSvc.EnsureBootstrapAdmin(ctx, employeeID, password)
	if err != nil {
		logger.Warnf(ctx,
			"[bootstrap] default-admin guarantee failed (will retry on next restart): %v", err)
		return
	}
	if !changed {
		return
	}
	if generatedPassword != "" {
		logger.Infof(ctx, "[bootstrap] ***********************************************************")
		logger.Infof(ctx, "[bootstrap] * default system admin created — employee ID: %s", employeeID)
		logger.Infof(ctx, "[bootstrap] * initial password (shown ONCE, not stored): %s", generatedPassword)
		logger.Infof(ctx, "[bootstrap] * you will be required to change it on first login")
		logger.Infof(ctx, "[bootstrap] ***********************************************************")
		return
	}
	logger.Infof(ctx,
		"[bootstrap] default system admin ready — employee ID: %s (password from %s, change required on first login)",
		employeeID, bootstrapAdminPasswordEnv)
}
