package handler

import (
	"github.com/gin-gonic/gin"
	"github.com/moistello/backend/internal/api/middleware"
	"github.com/moistello/backend/internal/domain/savings"
	"github.com/moistello/backend/pkg/response"
)

type SavingsGoalHandler struct {
	svc savings.Service
}

func NewSavingsGoalHandler(svc savings.Service) *SavingsGoalHandler {
	return &SavingsGoalHandler{svc: svc}
}

func (h *SavingsGoalHandler) Create(c *gin.Context) {
	userID := middleware.GetUserID(c)
	var req savings.CreateGoalRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	goal, err := h.svc.CreateGoal(c.Request.Context(), userID, req)
	if err != nil {
		response.InternalError(c, "failed to create savings goal")
		return
	}
	response.Created(c, gin.H{"goal": goal})
}

func (h *SavingsGoalHandler) List(c *gin.Context) {
	userID := middleware.GetUserID(c)
	goals, err := h.svc.ListGoals(c.Request.Context(), userID)
	if err != nil {
		response.InternalError(c, "failed to list savings goals")
		return
	}
	response.OK(c, gin.H{"goals": goals})
}

func (h *SavingsGoalHandler) ListActive(c *gin.Context) {
	userID := middleware.GetUserID(c)
	goals, err := h.svc.ListActiveGoals(c.Request.Context(), userID)
	if err != nil {
		response.InternalError(c, "failed to list active savings goals")
		return
	}
	response.OK(c, gin.H{"goals": goals})
}

func (h *SavingsGoalHandler) Get(c *gin.Context) {
	userID := middleware.GetUserID(c)
	goalID := c.Param("id")
	goal, err := h.svc.GetGoal(c.Request.Context(), userID, goalID)
	if err != nil {
		response.NotFound(c, "savings goal not found")
		return
	}
	response.OK(c, gin.H{"goal": goal})
}

func (h *SavingsGoalHandler) Update(c *gin.Context) {
	userID := middleware.GetUserID(c)
	goalID := c.Param("id")
	var req savings.UpdateGoalRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	goal, err := h.svc.UpdateGoal(c.Request.Context(), userID, goalID, req)
	if err != nil {
		response.InternalError(c, "failed to update savings goal")
		return
	}
	response.OK(c, gin.H{"goal": goal})
}

func (h *SavingsGoalHandler) Delete(c *gin.Context) {
	userID := middleware.GetUserID(c)
	goalID := c.Param("id")
	if err := h.svc.DeleteGoal(c.Request.Context(), userID, goalID); err != nil {
		response.NotFound(c, "savings goal not found")
		return
	}
	response.OK(c, gin.H{"success": true})
}

func (h *SavingsGoalHandler) Complete(c *gin.Context) {
	userID := middleware.GetUserID(c)
	goalID := c.Param("id")
	goal, err := h.svc.CompleteGoal(c.Request.Context(), userID, goalID)
	if err != nil {
		response.NotFound(c, "savings goal not found")
		return
	}
	response.OK(c, gin.H{"goal": goal})
}

func (h *SavingsGoalHandler) Summary(c *gin.Context) {
	userID := middleware.GetUserID(c)
	summary, err := h.svc.GetSummary(c.Request.Context(), userID)
	if err != nil {
		response.InternalError(c, "failed to get savings summary")
		return
	}
	response.OK(c, gin.H{"summary": summary})
}

func (h *SavingsGoalHandler) UpcomingObligations(c *gin.Context) {
	userID := middleware.GetUserID(c)
	goals, err := h.svc.GetUpcomingObligations(c.Request.Context(), userID)
	if err != nil {
		response.InternalError(c, "failed to get upcoming obligations")
		return
	}
	response.OK(c, gin.H{"goals": goals})
}
