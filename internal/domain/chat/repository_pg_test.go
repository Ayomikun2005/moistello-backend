package chat

import (
	"context"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/jmoiron/sqlx"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/moistello/backend/pkg/apperrors"
)

func newTestMsgRepo(t *testing.T) (Repository, sqlmock.Sqlmock, func()) {
	t.Helper()
	mockDB, mock, err := sqlmock.New()
	require.NoError(t, err)
	db := sqlx.NewDb(mockDB, "sqlmock")
	return NewRepository(db), mock, func() { mockDB.Close() }
}

func TestOrderPair_Deterministic(t *testing.T) {
	a, b := orderPair("zzz", "aaa")
	assert.Equal(t, "aaa", a)
	assert.Equal(t, "zzz", b)

	a2, b2 := orderPair("aaa", "zzz")
	assert.Equal(t, a, a2)
	assert.Equal(t, b, b2)
}

func TestRepository_GetOrCreateConversation_Existing(t *testing.T) {
	repo, mock, cleanup := newTestMsgRepo(t)
	defer cleanup()

	a, b := orderPair("user-x", "user-y")
	rows := sqlmock.NewRows([]string{"id", "user_a_id", "user_b_id", "created_at"}).
		AddRow("conv-1", a, b, time.Now())
	mock.ExpectQuery("SELECT id, user_a_id, user_b_id, created_at FROM chat_conversations WHERE user_a_id").
		WithArgs(a, b).
		WillReturnRows(rows)

	conv, err := repo.GetOrCreateConversation(context.Background(), "user-x", "user-y")
	require.NoError(t, err)
	assert.Equal(t, "conv-1", conv.ID)
}

func TestRepository_GetOrCreateConversation_CreatesWhenMissing(t *testing.T) {
	repo, mock, cleanup := newTestMsgRepo(t)
	defer cleanup()

	a, b := orderPair("user-x", "user-y")
	mock.ExpectQuery("SELECT id, user_a_id, user_b_id, created_at FROM chat_conversations WHERE user_a_id").
		WithArgs(a, b).
		WillReturnRows(sqlmock.NewRows([]string{"id", "user_a_id", "user_b_id", "created_at"}))
	mock.ExpectExec("INSERT INTO chat_conversations").
		WithArgs(a, b).
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectQuery("SELECT id, user_a_id, user_b_id, created_at FROM chat_conversations WHERE user_a_id").
		WithArgs(a, b).
		WillReturnRows(sqlmock.NewRows([]string{"id", "user_a_id", "user_b_id", "created_at"}).AddRow("conv-1", a, b, time.Now()))

	conv, err := repo.GetOrCreateConversation(context.Background(), "user-x", "user-y")
	require.NoError(t, err)
	assert.Equal(t, "conv-1", conv.ID)
}

func TestRepository_GetConversation_NotFound(t *testing.T) {
	repo, mock, cleanup := newTestMsgRepo(t)
	defer cleanup()

	mock.ExpectQuery("SELECT id, user_a_id, user_b_id, created_at FROM chat_conversations WHERE id").
		WithArgs("missing").
		WillReturnRows(sqlmock.NewRows([]string{"id", "user_a_id", "user_b_id", "created_at"}))

	_, err := repo.GetConversation(context.Background(), "missing")
	assert.ErrorIs(t, err, apperrors.ErrNotFound)
}

func TestRepository_CreateMessage_AssignsSequence(t *testing.T) {
	repo, mock, cleanup := newTestMsgRepo(t)
	defer cleanup()

	mock.ExpectBegin()
	mock.ExpectExec("SELECT id FROM chat_conversations WHERE id = (.+) FOR UPDATE").
		WithArgs("conv-1").
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery("SELECT COALESCE\\(MAX\\(sequence\\), 0\\) \\+ 1 FROM chat_messages").
		WithArgs("conv-1").
		WillReturnRows(sqlmock.NewRows([]string{"coalesce"}).AddRow(int64(3)))
	mock.ExpectExec("INSERT INTO chat_messages").
		WithArgs("msg-1", "conv-1", "user-a", int64(3), "ct", "n", nil, nil, sqlmock.AnyArg()).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	msg := &Message{ID: "msg-1", ConversationID: "conv-1", SenderID: "user-a", Ciphertext: "ct", Nonce: "n", CreatedAt: time.Now()}
	err := repo.CreateMessage(context.Background(), msg)
	require.NoError(t, err)
	assert.Equal(t, int64(3), msg.Sequence)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestRepository_ListMessages(t *testing.T) {
	repo, mock, cleanup := newTestMsgRepo(t)
	defer cleanup()

	rows := sqlmock.NewRows([]string{"id", "conversation_id", "sender_id", "sequence", "ciphertext", "nonce", "ephemeral_key", "one_time_prekey_id", "created_at"}).
		AddRow("msg-2", "conv-1", "user-a", 2, "ct2", "n2", nil, nil, time.Now()).
		AddRow("msg-1", "conv-1", "user-b", 1, "ct1", "n1", nil, nil, time.Now())
	mock.ExpectQuery("SELECT (.+) FROM chat_messages\\s+WHERE conversation_id = \\$1\\s+ORDER BY sequence DESC").
		WithArgs("conv-1", 50).
		WillReturnRows(rows)

	messages, err := repo.ListMessages(context.Background(), "conv-1", 0, 50)
	require.NoError(t, err)
	assert.Len(t, messages, 2)
}
