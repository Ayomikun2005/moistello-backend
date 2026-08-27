package handler_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/moistello/backend/internal/api/handler"
	"github.com/moistello/backend/internal/domain/circle"
	"github.com/moistello/backend/internal/domain/contribution"
	"github.com/moistello/backend/internal/domain/payout"
)

// stubs embed the domain interfaces so only the methods the handler needs
// are overridden.

type stubCircleService struct {
	circle.Service
	circle *circle.Circle
}

func (s *stubCircleService) Get(_ context.Context, id string) (*circle.Circle, error) {
	return s.circle, nil
}

type stubContribService struct {
	contribution.Service
	contribs []contribution.Contribution
	total    int
}

func (s *stubContribService) GetCircleHistory(_ context.Context, _ string, page, limit int) ([]contribution.Contribution, int, error) {
	return s.contribs, s.total, nil
}

type stubPayoutService struct {
	payout.Service
	payouts []payout.Payout
	total   int
}

func (s *stubPayoutService) GetCircleHistory(_ context.Context, _ string, page, limit int) ([]payout.Payout, int, error) {
	return s.payouts, s.total, nil
}

func TestCircleHandler_GetPayouts_AcceptsClientPagination(t *testing.T) {
	gin.SetMode(gin.TestMode)

	circleID := uuid.New()
	payouts := []payout.Payout{
		{ID: uuid.New(), CircleID: circleID, RoundNumber: 1, Amount: 100},
		{ID: uuid.New(), CircleID: circleID, RoundNumber: 2, Amount: 200},
	}

	svc := &stubPayoutService{payouts: payouts, total: 25}
	h := handler.NewCircleHandler(nil, nil, nil, svc)
	r := gin.New()
	r.GET("/circles/:id/payouts", h.GetPayouts)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/circles/"+circleID.String()+"/payouts?page=2&page_size=2", nil)
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)

	var body struct {
		Data struct {
			Payouts []payout.Payout `json:"payouts"`
		} `json:"data"`
		Meta struct {
			Page    int  `json:"page"`
			Limit   int  `json:"limit"`
			Total   int  `json:"total"`
			HasMore bool `json:"hasMore"`
		} `json:"meta"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	assert.Len(t, body.Data.Payouts, 2)
	assert.Equal(t, 2, body.Meta.Page)
	assert.Equal(t, 2, body.Meta.Limit)
	assert.Equal(t, 25, body.Meta.Total)
	assert.True(t, body.Meta.HasMore, "page 2 of 25 items at 2/page must have more pages")
}

func TestCircleHandler_GetRounds_AcceptsClientPaginationAndReturnsMeta(t *testing.T) {
	gin.SetMode(gin.TestMode)

	circleID := uuid.New()
	contribs := []contribution.Contribution{
		{ID: uuid.New(), CircleID: circleID, UserID: uuid.New(), RoundNumber: 1, Amount: 50},
	}
	payouts := []payout.Payout{
		{ID: uuid.New(), CircleID: circleID, RoundNumber: 1, Amount: 100},
	}

	h := handler.NewCircleHandler(
		&stubCircleService{circle: &circle.Circle{ID: circleID, CurrentRound: 1, MaxMembers: 4}},
		nil,
		&stubContribService{contribs: contribs, total: 3},
		&stubPayoutService{payouts: payouts, total: 3},
	)
	r := gin.New()
	r.GET("/circles/:id/rounds", h.GetRounds)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/circles/"+circleID.String()+"/rounds?page=1&page_size=1", nil)
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)

	var body struct {
		Data struct {
			Rounds []map[string]any `json:"rounds"`
		} `json:"data"`
		Meta struct {
			Page    int  `json:"page"`
			Limit   int  `json:"limit"`
			Total   int  `json:"total"`
			HasMore bool `json:"hasMore"`
		} `json:"meta"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	require.Len(t, body.Data.Rounds, 1)
	assert.Equal(t, float64(1), body.Data.Rounds[0]["roundNumber"])
	assert.Equal(t, 1, body.Meta.Page)
	assert.Equal(t, 1, body.Meta.Limit)
	assert.Equal(t, 3, body.Meta.Total)
	assert.True(t, body.Meta.HasMore)
}
