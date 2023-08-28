package healthcheck

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strconv"

	"github.com/samber/lo"
)

// Client can be used to query healthcheck information.
type Client struct {
	httpDo func(req *http.Request) (*http.Response, error)
}

func NewClient(client *http.Client) *Client {
	return &Client{
		httpDo: client.Do,
	}
}

// DiskUsage returns disk usage statistics or an error if unable to obtain.
// Do not include the port in the host.
func (c Client) DiskUsage(ctx context.Context, host string) ([]DiskUsageResponse, error) {
	var diskResps = make([]DiskUsageResponse, 0)
	u, err := url.Parse(host)
	if err != nil {
		return diskResps, fmt.Errorf("url parse: %w", err)
	}
	u.Host = net.JoinHostPort(u.Host, strconv.Itoa(Port))
	u.Path = "/disk"

	req, err := http.NewRequestWithContext(ctx, "GET", u.String(), nil)
	if err != nil {
		return diskResps, fmt.Errorf("new request: %w", err)
	}
	resp, err := c.httpDo(req)
	if err != nil {
		return diskResps, fmt.Errorf("http do: %w", err)
	}
	defer resp.Body.Close()
	if err = json.NewDecoder(resp.Body).Decode(&diskResps); err != nil {
		return diskResps, fmt.Errorf("malformed json: %w", err)
	}
	diskResps = lo.Filter(diskResps, func(item DiskUsageResponse, index int) bool {
		return item.Error == "" && item.AllBytes != 0
	})
	if len(diskResps) == 0 {
		return diskResps, errors.New("no disk usage data")
	}
	return diskResps, nil
}
