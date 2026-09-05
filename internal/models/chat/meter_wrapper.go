package chat

import (
	"context"

	"github.com/Tencent/WeKnora/internal/metering"
	"github.com/Tencent/WeKnora/internal/types"
)

// meterChat wraps a Chat and appends a usage-ledger record for every call
// (docs/Token统计与计费设计.md). Installed unconditionally in NewChat —
// unlike the Langfuse wrapper it must run even when tracing is off, and it
// never mutates the call itself. Tokens are counted regardless of business
// outcome: a failed call that already burned prompt/completion tokens is
// recorded with status=failed, and a stream cut short is recorded with
// status=interrupted plus estimated counts (extra.approx=true).
type meterChat struct {
	inner Chat
}

func wrapChatMeter(c Chat, err error) (Chat, error) {
	if err != nil || c == nil {
		return c, err
	}
	return &meterChat{inner: c}, nil
}

func (m *meterChat) GetModelName() string { return m.inner.GetModelName() }
func (m *meterChat) GetModelID() string   { return m.inner.GetModelID() }

func (m *meterChat) Chat(ctx context.Context, messages []Message, opts *ChatOptions) (*types.ChatResponse, error) {
	resp, err := m.inner.Chat(ctx, messages, opts)

	var usage types.TokenUsage
	if resp != nil {
		usage = resp.Usage
	}
	if usage.PromptTokens == 0 && usage.CompletionTokens == 0 && usage.TotalTokens == 0 {
		if err != nil || resp == nil {
			// Nothing measurable was consumed (or the provider reported
			// nothing on a failed call) — nothing to bill.
			return resp, err
		}
		// Provider succeeded but returned no usage (some OpenAI-compatible
		// endpoints): estimate so the ledger still counts the consumption.
		usage.PromptTokens = approxPromptTokens(messages)
		usage.CompletionTokens = metering.ApproxTokens(resp.Content)
		usage.TotalTokens = usage.PromptTokens + usage.CompletionTokens
		metering.GetManager().Submit(usageRecord(ctx, m.inner, messages,
			usage, statusFromErr(err), true))
		return resp, err
	}

	metering.GetManager().Submit(usageRecord(ctx, m.inner, messages,
		usage, statusFromErr(err), false))
	return resp, err
}

func (m *meterChat) ChatStream(ctx context.Context, messages []Message, opts *ChatOptions) (<-chan types.StreamResponse, error) {
	ch, err := m.inner.ChatStream(ctx, messages, opts)
	if err != nil {
		// The request never got a response going; nothing consumed.
		return ch, err
	}

	out := make(chan types.StreamResponse)
	go func() {
		defer close(out)
		var (
			usage       *types.TokenUsage
			contentRunes int
			sawDone      bool
		)
		forward := func(resp types.StreamResponse) bool {
			if resp.Usage != nil {
				usage = resp.Usage
			}
			if resp.Done {
				sawDone = true
			}
			contentRunes += len([]rune(resp.Content))
			select {
			case out <- resp:
				return true
			case <-ctx.Done():
				return false
			}
		}
		forwardAll := true
		for resp := range ch {
			if !forward(resp) {
				forwardAll = false
				break
			}
		}
		if !forwardAll {
			// Consumer went away mid-stream: keep draining the producer
			// side so it is not blocked on an unread channel.
			for range ch {
			}
		}

		var rec *types.ModelUsageRecord
		switch {
		case usage != nil:
			// Exact usage arrived on the stream (final chunk). err is not
			// visible here; Done marks a completed response.
			rec = usageRecord(ctx, m.inner, messages, *usage,
				statusFromDone(sawDone), false)
		case sawDone:
			// Completed but provider streamed no usage: estimate.
			u := estimatedStreamUsage(messages, contentRunes)
			rec = usageRecord(ctx, m.inner, messages, u, types.UsageStatusSuccess, true)
		default:
			// Stream ended without Done — interrupted. Count what was
			// actually sent: full prompt plus the partial answer.
			u := estimatedStreamUsage(messages, contentRunes)
			rec = usageRecord(ctx, m.inner, messages, u, types.UsageStatusInterrupted, true)
		}
		metering.GetManager().Submit(rec)
	}()
	return out, nil
}

// usageRecord assembles the ledger row shared by both paths.
func usageRecord(ctx context.Context, c Chat, messages []Message, usage types.TokenUsage, status string, approx bool) *types.ModelUsageRecord {
	if status == "" {
		status = types.UsageStatusSuccess
	}
	tenantID, userID, purpose := metering.AttributionFromContext(ctx)
	extra := types.JSONMap{}
	if approx {
		extra["approx"] = true
	}
	return &types.ModelUsageRecord{
		TenantID:      tenantID,
		UserID:        userID,
		ModelID:       c.GetModelID(),
		ModelName:     c.GetModelName(),
		Category:      types.UsageCategoryChat,
		Purpose:       purpose,
		Status:        status,
		InputTokens:   usage.PromptTokens,
		OutputTokens:  usage.CompletionTokens,
		CachedTokens:  usage.CacheReadTokens,
		Images:        countMessageImages(messages),
		Extra:         extra,
	}
}

func statusFromErr(err error) string {
	if err != nil {
		return types.UsageStatusFailed
	}
	return types.UsageStatusSuccess
}

func statusFromDone(sawDone bool) string {
	if sawDone {
		return types.UsageStatusSuccess
	}
	return types.UsageStatusInterrupted
}

// approxPromptTokens estimates the prompt size of a message list — text
// content only (image tokens are unknowable per-provider and are counted
// separately in extra.images), plus a small per-message envelope overhead.
func approxPromptTokens(messages []Message) int {
	total := 0
	for _, msg := range messages {
		total += metering.ApproxTokens(msg.Content)
		if msg.Content == "" && len(msg.MultiContent) > 0 {
			for _, part := range msg.MultiContent {
				if part.Type == "text" {
					total += metering.ApproxTokens(part.Text)
				}
			}
		}
		total += 6
	}
	return total
}

func approxRunes(runes int) int {
	if runes == 0 {
		return 0
	}
	return runes/4 + 1
}

// estimatedStreamUsage builds the fallback counts for streams that finish
// (or break) without a provider-reported usage chunk.
func estimatedStreamUsage(messages []Message, contentRunes int) types.TokenUsage {
	u := types.TokenUsage{
		PromptTokens:     approxPromptTokens(messages),
		CompletionTokens: approxRunes(contentRunes),
	}
	u.TotalTokens = u.PromptTokens + u.CompletionTokens
	return u
}

func countMessageImages(messages []Message) int {
	n := 0
	for _, msg := range messages {
		n += len(msg.Images)
		for _, part := range msg.MultiContent {
			if part.Type == "image_url" {
				n++
			}
		}
	}
	return n
}
