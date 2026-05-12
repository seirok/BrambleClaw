package sandbox

import "context"

type contextKey string

const sessionKeyKey contextKey = "sandbox.session_key"

// ContextWithSessionKey returns a new context carrying the session key.
func ContextWithSessionKey(ctx context.Context, sessionKey string) context.Context {
	return context.WithValue(ctx, sessionKeyKey, sessionKey)
}

// SessionKeyFromContext extracts the session key from context.
// Returns empty string if not present.
func SessionKeyFromContext(ctx context.Context) string {
	if v, ok := ctx.Value(sessionKeyKey).(string); ok {
		return v
	}
	return ""
}
