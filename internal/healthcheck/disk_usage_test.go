package healthcheck

import (
	"encoding/json"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestDiskUsage(t *testing.T) {
	t.Run("happy path", func(t *testing.T) {
		var (
			w       = httptest.NewRecorder()
			r       = httptest.NewRequest("GET", "/ignored", nil)
			handler = DiskUsage("tmp", "/")
		)
		handler(w, r)

		require.Equal(t, 200, w.Code)

		var resp []DiskUsageResponse
		err := json.Unmarshal(w.Body.Bytes(), &resp)
		got := resp[0]
		require.NoError(t, err)

		require.Equal(t, "/tmp", got.Dir)
		require.Equal(t, "tmp", got.PvcName)
		require.NotZero(t, got.AllBytes)
		require.NotZero(t, got.FreeBytes)
		require.True(t, got.AllBytes >= got.FreeBytes, "free bytes should not be more than all bytes")

		require.NotContains(t, w.Body.String(), "error")
	})

	t.Run("statfs error", func(t *testing.T) {
		const pvc = "this-directory-had-better-not-be-present-in-any-sort-of-test-environment"
		var (
			w       = httptest.NewRecorder()
			r       = httptest.NewRequest("GET", "/ignored", nil)
			handler = DiskUsage(pvc, "/")
		)
		handler(w, r)

		require.Equal(t, 500, w.Code)

		var resp []DiskUsageResponse
		err := json.Unmarshal(w.Body.Bytes(), &resp)
		got := resp[0]
		require.NoError(t, err)

		require.Equal(t, "/"+pvc, got.Dir)
		require.Equal(t, "no such file or directory", got.Error)
		require.NotContains(t, w.Body.String(), "all_bytes")
		require.NotContains(t, w.Body.String(), "free_bytes")
	})
}
