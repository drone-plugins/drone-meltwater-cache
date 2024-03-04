package harness

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"

	"github.com/go-kit/kit/log"
	"github.com/meltwater/drone-cache/harness"
	"github.com/meltwater/drone-cache/internal"
	"github.com/meltwater/drone-cache/storage/common"
)

type Backend struct {
	logger log.Logger
	token  string
	client harness.Client
}

// New creates an Harness backend.
func New(l log.Logger, c Config, debug bool) (*Backend, error) {
	cacheClient := harness.New(c.ServerBaseURL, c.AccountID, c.Token, false)
	backend := &Backend{
		logger: l,
		token:  c.Token,
		client: cacheClient,
	}
	return backend, nil
}

func (b *Backend) Get(ctx context.Context, key string, w io.Writer) error {
	preSignedURL, err := b.client.GetDownloadPresignURL(ctx, key)
	if err != nil {
		return err
	}
	res, err := b.do(ctx, "GET", preSignedURL, nil)
	if err != nil {
		return err
	}
	defer internal.CloseWithErrLogf(b.logger, res.Body, "response body, close defer")
	if res.StatusCode != http.StatusOK {
		return fmt.Errorf("received status code %d from presigned get url", res.StatusCode)
	}
	_, err = io.Copy(w, res.Body)
	if err != nil {
		return err
	}

	return nil
}

func (b *Backend) Put(ctx context.Context, key string, r io.Reader) error {
	preSignedURL, err := b.client.GetUploadPresignURL(ctx, key)
	if err != nil {
		return err
	}
	res, err := b.do(ctx, "PUT", preSignedURL, r)
	if err != nil {
		return err
	}
	defer internal.CloseWithErrLogf(b.logger, res.Body, "response body, close defer")
	if res.StatusCode != http.StatusOK {
		return fmt.Errorf("received status code %d from presigned put url", res.StatusCode)
	}

	return nil
}

func (b *Backend) Exists(ctx context.Context, key string) (bool, error) {
	preSignedURL, err := b.client.GetExistsPresignURL(ctx, key)
	if err != nil {
		return false, err
	}
	res, err := b.do(ctx, "HEAD", preSignedURL, nil)
	if err != nil {
		return false, nil
	}
	defer internal.CloseWithErrLogf(b.logger, res.Body, "response body, close defer")
	if res.StatusCode == http.StatusNotFound {
		return false, nil
	} else if res.StatusCode != http.StatusOK {
		return false, fmt.Errorf("unexpected status code %d", res.StatusCode)
	}

	return res.Header.Get("ETag") != "", nil
}

func (b *Backend) List(ctx context.Context, key string) ([]common.FileEntry, error) {
	// not implemented
	return nil, errors.New("list operation not implemented")
}

func (b *Backend) do(ctx context.Context, method, url string, body io.Reader) (*http.Response, error) {
	var (
		buffer []byte
		err    error
	)
	if body != nil {
		buffer, err = io.ReadAll(body)
		if err != nil {
			return nil, err
		}
	}
	req, err := http.NewRequestWithContext(ctx, method, url, bytes.NewReader(buffer))
	if err != nil {
		return nil, err
	}
	httpClient := http.Client{}
	res, err := httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	return res, nil
}
