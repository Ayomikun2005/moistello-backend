package response

import (
	"encoding/json"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestOKWithMetaAddsPaginationWithoutBreakingLegacyMeta(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)

	OKWithMeta(ctx, []string{"one"}, NewPaginationMeta(2, 20, 45))

	var body map[string]any
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &body))
	assert.NotNil(t, body["data"])

	meta := body["meta"].(map[string]any)
	assert.Equal(t, float64(20), meta["limit"])
	assert.Equal(t, float64(3), meta["totalPages"])
	assert.Equal(t, true, meta["hasMore"])

	current := body["pagination"].(map[string]any)
	assert.Equal(t, float64(2), current["page"])
	assert.Equal(t, float64(20), current["page_size"])
	assert.Equal(t, float64(45), current["total"])
	assert.Equal(t, float64(3), current["total_pages"])
	assert.Equal(t, true, current["has_more"])
}

func TestNewPaginationMeta_HasMore(t *testing.T) {
	// page 2 of 45 items at 20/page -> totalPages 3, so more pages remain.
	assert.True(t, NewPaginationMeta(2, 20, 45).HasMore)
	// Last page -> no more.
	assert.False(t, NewPaginationMeta(3, 20, 45).HasMore)
	// Empty collection -> no more.
	assert.False(t, NewPaginationMeta(1, 20, 0).HasMore)
}

func TestNewPaginationMetaEmptyCollectionHasZeroPages(t *testing.T) {
	meta := NewPaginationMeta(1, 20, 0)
	assert.Equal(t, 0, meta.TotalPages)
}
