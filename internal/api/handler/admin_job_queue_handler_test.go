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
	"github.com/moistello/backend/pkg/jobqueue"
)

// deadLetterJob enqueues a job and drives it into dead_letter status.
func deadLetterJob(t *testing.T, ctx context.Context, jq *jobqueue.JobQueue, queueName string) *jobqueue.Job {
	t.Helper()
	job, err := jq.Enqueue(ctx, queueName, "payload", 2)
	require.NoError(t, err)
	require.NoError(t, jq.Fail(ctx, job.ID, assert.AnError)) // retry 1 < max 2
	require.NoError(t, jq.Fail(ctx, job.ID, assert.AnError)) // retry 2 >= max 2 -> dead_letter
	return job
}

func TestAdminJobQueueHandler_GetDeadLetterJobs(t *testing.T) {
	gin.SetMode(gin.TestMode)
	ctx := context.Background()
	jq := jobqueue.NewJobQueue(nil)
	job := deadLetterJob(t, ctx, jq, "emails")

	h := handler.NewAdminJobQueueHandler(jq)
	r := gin.New()
	r.GET("/admin/jobs/dead-letter", h.GetDeadLetterJobs)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/admin/jobs/dead-letter", nil)
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	var body struct {
		Success bool `json:"success"`
		Data    struct {
			DeadLetterJobs []*jobqueue.Job `json:"dead_letter_jobs"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	require.Len(t, body.Data.DeadLetterJobs, 1)
	assert.Equal(t, job.ID, body.Data.DeadLetterJobs[0].ID)
	assert.Equal(t, jobqueue.StatusDeadLetter, body.Data.DeadLetterJobs[0].Status)
}

func TestAdminJobQueueHandler_RetryDeadLetterJob(t *testing.T) {
	gin.SetMode(gin.TestMode)
	ctx := context.Background()
	jq := jobqueue.NewJobQueue(nil)
	job := deadLetterJob(t, ctx, jq, "emails")

	h := handler.NewAdminJobQueueHandler(jq)
	r := gin.New()
	r.POST("/admin/jobs/dead-letter/:id/retry", h.RetryDeadLetterJob)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("POST", "/admin/jobs/dead-letter/"+job.ID+"/retry", nil)
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	var body struct {
		Success bool `json:"success"`
		Data    struct {
			Message string `json:"message"`
			JobID   string `json:"job_id"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	assert.Contains(t, body.Data.Message, "requeued")
	assert.Equal(t, job.ID, body.Data.JobID)

	// The job is back to pending and retries reset.
	retried, err := jq.GetDeadLetterJobs(ctx)
	require.NoError(t, err)
	require.Len(t, retried, 0, "job should no longer be in dead_letter after retry")
}

func TestAdminJobQueueHandler_RetryDeadLetterJob_UnknownID(t *testing.T) {
	gin.SetMode(gin.TestMode)
	jq := jobqueue.NewJobQueue(nil)

	h := handler.NewAdminJobQueueHandler(jq)
	r := gin.New()
	r.POST("/admin/jobs/dead-letter/:id/retry", h.RetryDeadLetterJob)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("POST", "/admin/jobs/dead-letter/does-not-exist/retry", nil)
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}
