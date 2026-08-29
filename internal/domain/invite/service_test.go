package invite_test

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"

	"github.com/moistello/backend/internal/domain/invite"
	inviteMocks "github.com/moistello/backend/internal/domain/invite/mocks"
	"github.com/moistello/backend/pkg/apperrors"
)

func TestService_Generate_Success(t *testing.T) {
	repo := new(inviteMocks.Repository)
	svc := invite.NewService(repo)

	circleID := uuid.New().String()
	userID := uuid.New().String()
	input := invite.GenerateInput{
		CircleID: circleID,
		UserID:   userID,
		MaxUses:  10,
	}

	repo.On("Create", mock.Anything, mock.AnythingOfType("*invite.Invite")).Return(nil)

	result, err := svc.Generate(context.Background(), input)

	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.NotEmpty(t, result.Code)
	assert.Equal(t, 10, result.MaxUses)
	assert.Equal(t, 0, result.UseCount)
	assert.NotEqual(t, uuid.Nil, result.ID)
	repo.AssertExpectations(t)
}

func TestService_Generate_WithTTL(t *testing.T) {
	repo := new(inviteMocks.Repository)
	svc := invite.NewService(repo)

	circleID := uuid.New().String()
	userID := uuid.New().String()
	input := invite.GenerateInput{
		CircleID: circleID,
		UserID:   userID,
		MaxUses:  5,
		TTLHours: 24,
	}

	repo.On("Create", mock.Anything, mock.AnythingOfType("*invite.Invite")).Return(nil)

	result, err := svc.Generate(context.Background(), input)

	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.True(t, result.ExpiresAt.Valid, "ExpiresAt should be set when TTLHours > 0")
	// Verify the expiry is approximately 24 hours from now
	expectedExpiry := time.Now().UTC().Add(24 * time.Hour)
	assert.WithinDuration(t, expectedExpiry, result.ExpiresAt.Time, 5*time.Second)
	repo.AssertExpectations(t)
}

func TestService_Generate_WithoutTTL(t *testing.T) {
	repo := new(inviteMocks.Repository)
	svc := invite.NewService(repo)

	input := invite.GenerateInput{
		CircleID: uuid.New().String(),
		UserID:   uuid.New().String(),
		MaxUses:  5,
		TTLHours: 0,
	}

	repo.On("Create", mock.Anything, mock.AnythingOfType("*invite.Invite")).Return(nil)

	result, err := svc.Generate(context.Background(), input)

	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.False(t, result.ExpiresAt.Valid, "ExpiresAt should not be set when TTLHours is 0")
	repo.AssertExpectations(t)
}

func TestService_Generate_InvalidCircleID(t *testing.T) {
	repo := new(inviteMocks.Repository)
	svc := invite.NewService(repo)

	input := invite.GenerateInput{
		CircleID: "not-a-uuid",
		UserID:   uuid.New().String(),
		MaxUses:  5,
	}

	result, err := svc.Generate(context.Background(), input)

	assert.Error(t, err)
	assert.Nil(t, result)
	assert.Contains(t, err.Error(), "invalid UUID")
}

func TestService_Generate_InvalidUserID(t *testing.T) {
	repo := new(inviteMocks.Repository)
	svc := invite.NewService(repo)

	input := invite.GenerateInput{
		CircleID: uuid.New().String(),
		UserID:   "not-a-uuid",
		MaxUses:  5,
	}

	result, err := svc.Generate(context.Background(), input)

	assert.Error(t, err)
	assert.Nil(t, result)
	assert.Contains(t, err.Error(), "invalid UUID")
}

func TestService_Generate_RepoError(t *testing.T) {
	repo := new(inviteMocks.Repository)
	svc := invite.NewService(repo)

	input := invite.GenerateInput{
		CircleID: uuid.New().String(),
		UserID:   uuid.New().String(),
		MaxUses:  5,
	}

	repo.On("Create", mock.Anything, mock.AnythingOfType("*invite.Invite")).Return(assert.AnError)

	result, err := svc.Generate(context.Background(), input)

	assert.Error(t, err)
	assert.Nil(t, result)
	assert.Contains(t, err.Error(), "generating invite")
	repo.AssertExpectations(t)
}

func TestService_Validate_Success(t *testing.T) {
	repo := new(inviteMocks.Repository)
	svc := invite.NewService(repo)

	inv := &invite.Invite{
		ID:       uuid.New(),
		CircleID: uuid.New(),
		Code:     "abc123",
		MaxUses:  10,
		UseCount: 3,
	}

	repo.On("FindByCode", mock.Anything, "abc123").Return(inv, nil)

	result, err := svc.Validate(context.Background(), "abc123")

	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.Equal(t, "abc123", result.Code)
	repo.AssertExpectations(t)
}

func TestService_Validate_NotFound(t *testing.T) {
	repo := new(inviteMocks.Repository)
	svc := invite.NewService(repo)

	repo.On("FindByCode", mock.Anything, "nonexistent").Return(nil, apperrors.ErrNotFound)

	result, err := svc.Validate(context.Background(), "nonexistent")

	assert.Error(t, err)
	assert.Nil(t, result)
	assert.ErrorIs(t, err, apperrors.ErrInvalidInvite)
	repo.AssertExpectations(t)
}

func TestService_Validate_MaxUsesExceeded(t *testing.T) {
	repo := new(inviteMocks.Repository)
	svc := invite.NewService(repo)

	inv := &invite.Invite{
		ID:       uuid.New(),
		CircleID: uuid.New(),
		Code:     "maxed-out",
		MaxUses:  5,
		UseCount: 5, // At max
	}

	repo.On("FindByCode", mock.Anything, "maxed-out").Return(inv, nil)

	result, err := svc.Validate(context.Background(), "maxed-out")

	assert.Error(t, err)
	assert.Nil(t, result)
	assert.ErrorIs(t, err, apperrors.ErrInvalidInvite)
	repo.AssertExpectations(t)
}

func TestService_Validate_Expired(t *testing.T) {
	repo := new(inviteMocks.Repository)
	svc := invite.NewService(repo)

	inv := &invite.Invite{
		ID:       uuid.New(),
		CircleID: uuid.New(),
		Code:     "expired-code",
		MaxUses:  10,
		UseCount: 0,
		ExpiresAt: sql.NullTime{
			Time:  time.Now().UTC().Add(-1 * time.Hour), // Expired 1 hour ago
			Valid: true,
		},
	}

	repo.On("FindByCode", mock.Anything, "expired-code").Return(inv, nil)

	result, err := svc.Validate(context.Background(), "expired-code")

	assert.Error(t, err)
	assert.Nil(t, result)
	assert.ErrorIs(t, err, apperrors.ErrInvalidInvite)
	repo.AssertExpectations(t)
}

func TestService_Validate_NoExpiry_NoMaxUses(t *testing.T) {
	repo := new(inviteMocks.Repository)
	svc := invite.NewService(repo)

	inv := &invite.Invite{
		ID:        uuid.New(),
		CircleID:  uuid.New(),
		Code:      "unlimited",
		MaxUses:   0, // No limit
		UseCount:  100,
		ExpiresAt: sql.NullTime{Valid: false}, // No expiry
	}

	repo.On("FindByCode", mock.Anything, "unlimited").Return(inv, nil)

	result, err := svc.Validate(context.Background(), "unlimited")

	assert.NoError(t, err)
	assert.NotNil(t, result)
	repo.AssertExpectations(t)
}

func TestService_List_Success(t *testing.T) {
	repo := new(inviteMocks.Repository)
	svc := invite.NewService(repo)

	circleID := uuid.New()
	expected := []invite.Invite{
		{ID: uuid.New(), CircleID: circleID, Code: "code1"},
		{ID: uuid.New(), CircleID: circleID, Code: "code2"},
	}

	repo.On("FindByCircle", mock.Anything, circleID).Return(expected, nil)

	results, err := svc.List(context.Background(), circleID.String())

	assert.NoError(t, err)
	assert.Len(t, results, 2)
	assert.Equal(t, "code1", results[0].Code)
	repo.AssertExpectations(t)
}

func TestService_List_InvalidCircleID(t *testing.T) {
	repo := new(inviteMocks.Repository)
	svc := invite.NewService(repo)

	results, err := svc.List(context.Background(), "not-a-uuid")

	assert.Error(t, err)
	assert.Nil(t, results)
	assert.Contains(t, err.Error(), "invalid UUID")
}

func TestService_Revoke_Success(t *testing.T) {
	repo := new(inviteMocks.Repository)
	svc := invite.NewService(repo)

	inviteID := uuid.New()
	repo.On("Delete", mock.Anything, inviteID).Return(nil)

	err := svc.Revoke(context.Background(), inviteID.String(), uuid.New().String())

	assert.NoError(t, err)
	repo.AssertExpectations(t)
}

func TestService_Revoke_NotFound(t *testing.T) {
	repo := new(inviteMocks.Repository)
	svc := invite.NewService(repo)

	inviteID := uuid.New()
	repo.On("Delete", mock.Anything, inviteID).Return(apperrors.ErrNotFound)

	err := svc.Revoke(context.Background(), inviteID.String(), uuid.New().String())

	assert.Error(t, err)
	assert.ErrorIs(t, err, apperrors.ErrInvalidInvite)
	repo.AssertExpectations(t)
}

func TestService_Revoke_InvalidID(t *testing.T) {
	repo := new(inviteMocks.Repository)
	svc := invite.NewService(repo)

	err := svc.Revoke(context.Background(), "bad-id", uuid.New().String())

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "invalid UUID")
}
