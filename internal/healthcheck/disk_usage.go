package healthcheck

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"path/filepath"
	"strings"
	"syscall"

	"github.com/samber/lo"
)

// DiskUsageResponse returns disk statistics in bytes.
type DiskUsageResponse struct {
	Dir       string `json:"dir"`
	PvcName   string `json:"pvc_name"`
	AllBytes  uint64 `json:"all_bytes,omitempty"`
	FreeBytes uint64 `json:"free_bytes,omitempty"`
	Error     string `json:"error,omitempty"`
}

// DiskUsage returns a handler which responds with disk statistics in JSON.
// Path is the filesystem path from which to check disk usage.
func DiskUsage(pvcs string, mount string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var resps = make([]DiskUsageResponse, 0)
		pvcList := lo.Filter(lo.Uniq(strings.Split(pvcs, ",")), func(name string, _ int) bool {
			return name != ""
		})
		var merr error

		if len(pvcList) == 0 {
			w.WriteHeader(http.StatusInternalServerError)
			mustJSONEncode(resps, w)
			return
		}

		for _, pvc := range pvcList {
			// it should be mounted on /mnt/<pvc>
			dir := filepath.Clean(mount + "/" + pvc)
			var resp DiskUsageResponse

			resp.Dir = dir
			resp.PvcName = pvc
			var fs syscall.Statfs_t
			// Purposefully not adding test hook, so tests may catch OS issues.
			err := syscall.Statfs(dir, &fs)
			if err != nil {
				resp.Error = err.Error()
				resps = append(resps, resp)
				merr = errors.Join(merr, err)

				continue
			}

			all := fs.Blocks * uint64(fs.Bsize)
			free := fs.Bfree * uint64(fs.Bsize)

			resp.AllBytes = all
			resp.FreeBytes = free

			resps = append(resps, resp)
		}

		if merr != nil {
			w.WriteHeader(http.StatusInternalServerError)
			mustJSONEncode(resps, w)
			return
		}

		w.WriteHeader(http.StatusOK)
		mustJSONEncode(resps, w)
	}
}

func mustJSONEncode(v interface{}, w io.Writer) {
	if err := json.NewEncoder(w).Encode(v); err != nil {
		panic(err)
	}
}
