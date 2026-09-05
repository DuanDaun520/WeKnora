package metering

import (
	"context"

	"github.com/Tencent/WeKnora/internal/types"
)

// AttributionFromContext reads the billing identity the middleware and
// orchestration layers attach to every request: the space (tenant), the
// acting user (empty for system/background tasks) and the LLM call
// purpose scene label (agent_round, document_summary, ...).
func AttributionFromContext(ctx context.Context) (tenantID uint64, userID, purpose string) {
	if ctx == nil {
		return 0, "", ""
	}
	if v, ok := ctx.Value(types.TenantIDContextKey).(uint64); ok {
		tenantID = v
	}
	if v, ok := ctx.Value(types.UserIDContextKey).(string); ok {
		userID = v
	}
	purpose, _ = types.LLMCallMetadataFromContext(ctx)
	return tenantID, userID, purpose
}

// ApproxTokens estimates token count as ~rune_count/4 + 1 per string,
// matching the rule the Langfuse embedding wrapper uses
// (approxEmbeddingUsage). Keeping both estimators on the same rule lets
// the two systems' numbers be compared side by side. Approximations must
// be flagged with extra.approx = true by the caller.
func ApproxTokens(s string) int {
	runes := len([]rune(s))
	if runes == 0 {
		return 0
	}
	return runes/4 + 1
}
