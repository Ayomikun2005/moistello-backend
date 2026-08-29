package logger

import (
	"bytes"
	"context"
	"testing"

	"github.com/rs/zerolog/log"
	"github.com/stretchr/testify/require"
)

func TestCtx_IncludesRequestIDAndUserID(t *testing.T) {
	var buf bytes.Buffer
	original := log.Logger
	log.Logger = log.Output(&buf)
	t.Cleanup(func() { log.Logger = original })

	ctx := context.WithValue(context.Background(), RequestIDKey, "req-123")
	ctx = context.WithValue(ctx, UserIDKey, "user-456")

	l := Ctx(ctx)
	l.Info().Msg("hello")

	out := buf.String()
	require.Contains(t, out, "requestID")
	require.Contains(t, out, "req-123")
	require.Contains(t, out, "userID")
	require.Contains(t, out, "user-456")
}

func TestCtx_NoContextFieldsWhenAbsent(t *testing.T) {
	var buf bytes.Buffer
	original := log.Logger
	log.Logger = log.Output(&buf)
	t.Cleanup(func() { log.Logger = original })

	l := Ctx(context.Background())
	l.Info().Msg("bare")

	out := buf.String()
	require.NotContains(t, out, "requestID")
	require.NotContains(t, out, "userID")
}
