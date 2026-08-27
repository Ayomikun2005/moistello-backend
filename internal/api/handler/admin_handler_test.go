package handler_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/moistello/backend/internal/api/handler"
	"github.com/moistello/backend/internal/domain/admin"
)

type fakeMetricsRepo struct {
	metrics *admin.Metrics
}

func (f *fakeMetricsRepo) Metrics(_ context.Context, _ int) (*admin.Metrics, error) {
	return f.metrics, nil
}

func (f *fakeMetricsRepo) DailyVolume(_ context.Context, _ int) ([]admin.DailyVolumePoint, error) {
	return f.metrics.DailyVolume, nil
}

func TestAdminHandler_GetMetrics_ReturnsRealAggregates(t *testing.T) {
	gin.SetMode(gin.TestMode)

	svc := admin.NewService(&fakeMetricsRepo{metrics: &admin.Metrics{
		TotalUsers:         10,
		TotalCircles:       5,
		ActiveCircles:      2,
		TotalContributions: 40,
		TotalPayouts:       5,
		ActiveUsers:        8,
		NewUsers30d:        3,
		ContributionVolume: 100.5,
		PayoutVolume:       50.25,
		TotalVolumeUSD:     150.75,
		VolumeUSD30d:       20,
		DailyVolume: []admin.DailyVolumePoint{
			{ContributionVolume: 10, PayoutVolume: 5},
		},
	}}, 0)

	h := handler.NewAdminHandler(nil, nil, nil, nil, svc)
	r := gin.New()
	r.GET("/admin/metrics", h.GetMetrics)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/admin/metrics?days=7", nil)
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)

	var body struct {
		Data struct {
			TotalUsers         int                      `json:"totalUsers"`
			ActiveCircles      int                      `json:"activeCircles"`
			TotalVolumeUSD     float64                  `json:"totalVolumeUSD"`
			ContributionVolume float64                  `json:"contributionVolume"`
			DailyVolume        []admin.DailyVolumePoint `json:"dailyVolume"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))

	assert.Equal(t, 10, body.Data.TotalUsers)
	assert.Equal(t, 2, body.Data.ActiveCircles)
	assert.Equal(t, 150.75, body.Data.TotalVolumeUSD)
	assert.NotZero(t, body.Data.TotalVolumeUSD, "totalVolumeUSD must no longer be hardcoded to 0")
	assert.Equal(t, 100.5, body.Data.ContributionVolume)
	assert.Len(t, body.Data.DailyVolume, 1)
	assert.Equal(t, 10.0, body.Data.DailyVolume[0].ContributionVolume)
}
