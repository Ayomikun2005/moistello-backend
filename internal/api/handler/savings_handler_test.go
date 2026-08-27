package handler_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"

	"github.com/moistello/backend/internal/api/handler"
	"github.com/moistello/backend/internal/domain/savings"
	savingsMocks "github.com/moistello/backend/internal/domain/savings/mocks"
)

func TestSavingsHandler_Create_Valid(t *testing.T) {
	gin.SetMode(gin.TestMode)

	repo := new(savingsMocks.Repository)
	svc := savings.NewService(repo)

	repo.On("Create", mock.Anything, mock.AnythingOfType("*savings.SavingsGoal")).Return(nil).Run(func(args mock.Arguments) {
		g := args.Get(1).(*savings.SavingsGoal)
		g.ID = "goal-1"
	})

	h := handler.NewSavingsGoalHandler(svc)
	r := gin.New()
	r.Use(func(c *gin.Context) { c.Set("userID", "user-1"); c.Next() })
	r.POST("/savings/goals", h.Create)

	body, _ := json.Marshal(map[string]interface{}{
		"name": "Trip", "description": "Summer", "targetAmount": 2000, "autoReserve": true,
	})
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("POST", "/savings/goals", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusCreated, w.Code)
	assert.Contains(t, w.Body.String(), "Trip")
	repo.AssertExpectations(t)
}

func TestSavingsHandler_Create_MissingName(t *testing.T) {
	gin.SetMode(gin.TestMode)

	repo := new(savingsMocks.Repository)
	svc := savings.NewService(repo)
	h := handler.NewSavingsGoalHandler(svc)
	r := gin.New()
	r.Use(func(c *gin.Context) { c.Set("userID", "user-1"); c.Next() })
	r.POST("/savings/goals", h.Create)

	body, _ := json.Marshal(map[string]interface{}{"targetAmount": 2000})
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("POST", "/savings/goals", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
	repo.AssertNotCalled(t, "Create", mock.Anything, mock.Anything)
}

func TestSavingsHandler_List(t *testing.T) {
	gin.SetMode(gin.TestMode)

	repo := new(savingsMocks.Repository)
	svc := savings.NewService(repo)

	repo.On("FindByUserID", mock.Anything, "user-1").Return([]savings.SavingsGoal{{ID: "goal-1", UserID: "user-1", Name: "A"}}, nil)

	h := handler.NewSavingsGoalHandler(svc)
	r := gin.New()
	r.Use(func(c *gin.Context) { c.Set("userID", "user-1"); c.Next() })
	r.GET("/savings/goals", h.List)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/savings/goals", nil)
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), "goal-1")
	repo.AssertExpectations(t)
}

func TestSavingsHandler_Get_Owned(t *testing.T) {
	gin.SetMode(gin.TestMode)

	repo := new(savingsMocks.Repository)
	svc := savings.NewService(repo)

	repo.On("FindByID", mock.Anything, "goal-1").Return(&savings.SavingsGoal{ID: "goal-1", UserID: "user-1", Name: "Trip"}, nil)

	h := handler.NewSavingsGoalHandler(svc)
	r := gin.New()
	r.Use(func(c *gin.Context) { c.Set("userID", "user-1"); c.Next() })
	r.GET("/savings/goals/:id", h.Get)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/savings/goals/goal-1", nil)
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), "Trip")
	repo.AssertExpectations(t)
}

func TestSavingsHandler_Get_NotOwned_NotFound(t *testing.T) {
	gin.SetMode(gin.TestMode)

	repo := new(savingsMocks.Repository)
	svc := savings.NewService(repo)

	repo.On("FindByID", mock.Anything, "goal-1").Return(&savings.SavingsGoal{ID: "goal-1", UserID: "user-2"}, nil)

	h := handler.NewSavingsGoalHandler(svc)
	r := gin.New()
	r.Use(func(c *gin.Context) { c.Set("userID", "user-1"); c.Next() })
	r.GET("/savings/goals/:id", h.Get)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/savings/goals/goal-1", nil)
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNotFound, w.Code)
	repo.AssertExpectations(t)
}

func TestSavingsHandler_Delete_Owned(t *testing.T) {
	gin.SetMode(gin.TestMode)

	repo := new(savingsMocks.Repository)
	svc := savings.NewService(repo)

	repo.On("FindByID", mock.Anything, "goal-1").Return(&savings.SavingsGoal{ID: "goal-1", UserID: "user-1"}, nil)
	repo.On("Delete", mock.Anything, "goal-1").Return(nil)

	h := handler.NewSavingsGoalHandler(svc)
	r := gin.New()
	r.Use(func(c *gin.Context) { c.Set("userID", "user-1"); c.Next() })
	r.DELETE("/savings/goals/:id", h.Delete)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("DELETE", "/savings/goals/goal-1", nil)
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), `"success":true`)
	repo.AssertExpectations(t)
}

func TestSavingsHandler_Delete_NotOwned(t *testing.T) {
	gin.SetMode(gin.TestMode)

	repo := new(savingsMocks.Repository)
	svc := savings.NewService(repo)

	repo.On("FindByID", mock.Anything, "goal-1").Return(&savings.SavingsGoal{ID: "goal-1", UserID: "user-2"}, nil)

	h := handler.NewSavingsGoalHandler(svc)
	r := gin.New()
	r.Use(func(c *gin.Context) { c.Set("userID", "user-1"); c.Next() })
	r.DELETE("/savings/goals/:id", h.Delete)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("DELETE", "/savings/goals/goal-1", nil)
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNotFound, w.Code)
	repo.AssertExpectations(t)
}

func TestSavingsHandler_Complete_Owned(t *testing.T) {
	gin.SetMode(gin.TestMode)

	repo := new(savingsMocks.Repository)
	svc := savings.NewService(repo)

	repo.On("FindByID", mock.Anything, "goal-1").Return(&savings.SavingsGoal{ID: "goal-1", UserID: "user-1", TargetAmount: 500}, nil)
	repo.On("Update", mock.Anything, mock.AnythingOfType("*savings.SavingsGoal")).Return(nil)

	h := handler.NewSavingsGoalHandler(svc)
	r := gin.New()
	r.Use(func(c *gin.Context) { c.Set("userID", "user-1"); c.Next() })
	r.POST("/savings/goals/:id/complete", h.Complete)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("POST", "/savings/goals/goal-1/complete", nil)
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), `"status":"completed`)
	repo.AssertExpectations(t)
}

func TestSavingsHandler_Summary(t *testing.T) {
	gin.SetMode(gin.TestMode)

	repo := new(savingsMocks.Repository)
	svc := savings.NewService(repo)

	repo.On("GetSummary", mock.Anything, "user-1").Return(&savings.GoalSummary{TotalGoals: 2, ActiveGoals: 1}, nil)

	h := handler.NewSavingsGoalHandler(svc)
	r := gin.New()
	r.Use(func(c *gin.Context) { c.Set("userID", "user-1"); c.Next() })
	r.GET("/savings/goals/summary", h.Summary)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/savings/goals/summary", nil)
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), `"totalGoals":2`)
	repo.AssertExpectations(t)
}
