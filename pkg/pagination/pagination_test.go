package pagination

import (
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
)

func parseQuery(query string) (int, int, int) {
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	ctx.Request = httptest.NewRequest("GET", "/items?"+query, nil)
	return Parse(ctx)
}

func TestParseDefaultsAndBounds(t *testing.T) {
	gin.SetMode(gin.TestMode)

	page, size, offset := parseQuery("")
	assert.Equal(t, 1, page)
	assert.Equal(t, 20, size)
	assert.Equal(t, 0, offset)

	page, size, offset = parseQuery("page=3&page_size=250")
	assert.Equal(t, 3, page)
	assert.Equal(t, 100, size)
	assert.Equal(t, 200, offset)
}

func TestParseSupportsLegacyLimitAndPrefersPageSize(t *testing.T) {
	_, size, _ := parseQuery("limit=40")
	assert.Equal(t, 40, size)

	_, size, _ = parseQuery("limit=40&page_size=15")
	assert.Equal(t, 15, size)
}
