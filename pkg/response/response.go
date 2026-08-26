package response

import (
	"context"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/rs/zerolog/log"
	"go.opentelemetry.io/otel/trace"
)

type PaginationMeta struct {
	Page       int  `json:"page"`
	Limit      int  `json:"limit"`
	Total      int  `json:"total"`
	TotalPages int  `json:"totalPages"`
	HasMore    bool `json:"hasMore"`
}

// Pagination is the stable list-response contract. PaginationMeta remains in
// the envelope for clients using the original camelCase API.
type Pagination struct {
	Page       int  `json:"page"`
	PageSize   int  `json:"page_size"`
	Total      int  `json:"total"`
	TotalPages int  `json:"total_pages"`
	HasMore    bool `json:"has_more"`
}

type Envelope = APIResponse

type APIResponse struct {
	Success    bool            `json:"success"`
	Data       any             `json:"data,omitempty"`
	Error      string          `json:"error,omitempty"`
	Code       string          `json:"code,omitempty"`
	StatusCode int             `json:"statusCode,omitempty"`
	RequestID  string          `json:"requestId,omitempty"`
	Meta       *PaginationMeta `json:"meta,omitempty"`
	Pagination *Pagination     `json:"pagination,omitempty"`
}

func getReqID(c *gin.Context) string {
	if reqID, exists := c.Get("requestID"); exists {
		if s, ok := reqID.(string); ok {
			return s
		}
	}
	return c.GetHeader("X-Request-ID")
}

func ErrorWithCode(c *gin.Context, statusCode int, code, msg string) {
	errorResponse(c, statusCode, code, msg, nil)
}

// Error responds with an error message derived from the given error. The
// underlying error is logged (structured, requestID-correlated); only msg is
// returned to the client.
func Error(c *gin.Context, statusCode int, msg string, err error) {
	errorResponse(c, statusCode, "ERROR", msg, err)
}

// errorResponse writes the error envelope and emits a structured log line that
// ties the error response back to the originating request via its requestID.
func errorResponse(c *gin.Context, statusCode int, code, msg string, underlying error) {
	requestID := getReqID(c)

	// Emit a structured, requestID-correlated log so an error response can be
	// traced back to the exact request that produced it (#229).
	evt := log.Warn()
	if statusCode >= http.StatusInternalServerError {
		evt = log.Error()
	}
	evt.
		Str("requestID", requestID).
		Str("method", c.Request.Method).
		Str("path", c.Request.URL.Path).
		Int("status", statusCode).
		Str("code", code).
		Str("message", msg).
		Str("trace_id", traceIDFromContext(c.Request.Context()))
	if underlying != nil {
		evt = evt.Err(underlying)
	}
	evt.Msg("request error")

	c.JSON(statusCode, APIResponse{
		Success:    false,
		Error:      msg,
		Code:       code,
		StatusCode: statusCode,
		RequestID:  requestID,
	})
}

func traceIDFromContext(ctx context.Context) string {
	spanCtx := trace.SpanFromContext(ctx).SpanContext()
	if spanCtx.HasTraceID() {
		return spanCtx.TraceID().String()
	}
	return ""
}

// Success responds with a success envelope carrying data.
func Success(c *gin.Context, data any) {
	c.JSON(http.StatusOK, APIResponse{Success: true, Data: data})
}

func OK(c *gin.Context, data any) {
	c.JSON(http.StatusOK, APIResponse{Success: true, Data: data})
}

func OKWithMeta(c *gin.Context, data any, meta *PaginationMeta) {
	var current *Pagination
	if meta != nil {
		current = &Pagination{
			Page: meta.Page, PageSize: meta.Limit, Total: meta.Total, TotalPages: meta.TotalPages, HasMore: meta.HasMore,
		}
	}
	c.JSON(http.StatusOK, APIResponse{
		Success: true, Data: data, Meta: meta, Pagination: current,
	})
}

func Created(c *gin.Context, data any) {
	c.JSON(http.StatusCreated, APIResponse{Success: true, Data: data})
}

func BadRequest(c *gin.Context, msg string) {
	ErrorWithCode(c, http.StatusBadRequest, "BAD_REQUEST", msg)
}

func Unauthorized(c *gin.Context, msg string) {
	ErrorWithCode(c, http.StatusUnauthorized, "UNAUTHORIZED", msg)
}

func Forbidden(c *gin.Context, msg string) {
	ErrorWithCode(c, http.StatusForbidden, "FORBIDDEN", msg)
}

func NotFound(c *gin.Context, msg string) {
	ErrorWithCode(c, http.StatusNotFound, "NOT_FOUND", msg)
}

func Conflict(c *gin.Context, msg string) {
	ErrorWithCode(c, http.StatusConflict, "CONFLICT", msg)
}

func InternalError(c *gin.Context, msg string) {
	ErrorWithCode(c, http.StatusInternalServerError, "INTERNAL_ERROR", msg)
}

func ValidationErrors(c *gin.Context, msg string) {
	ErrorWithCode(c, http.StatusUnprocessableEntity, "VALIDATION_ERROR", msg)
}

func NewPaginationMeta(page, limit, total int) *PaginationMeta {
	if page < 1 {
		page = 1
	}
	if limit < 1 {
		limit = 20
	}
	totalPages := (total + limit - 1) / limit
	return &PaginationMeta{Page: page, Limit: limit, Total: total, TotalPages: totalPages, HasMore: page < totalPages}
}
