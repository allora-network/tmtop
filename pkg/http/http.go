package http

import (
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"path"
	"time"

	"github.com/rs/zerolog"
)

type Client struct {
	Logger zerolog.Logger
	Host   string
}

func NewClient(logger zerolog.Logger, invoker, host string) *Client {
	return &Client{
		Logger: logger.With().
			Str("component", "http").
			Str("invoker", invoker).
			Logger(),
		Host: host,
	}
}

func (c *Client) join(host, rest string) string {
	base, _ := url.Parse(host)
	ref, _ := url.Parse(rest)

	base.Path = path.Join(base.Path, ref.Path)
	return base.String()
}

func (c *Client) GetInternal(relativeURL string) (io.ReadCloser, error) {
	var transport http.RoundTripper

	transportRaw, ok := http.DefaultTransport.(*http.Transport)
	if ok {
		transport = transportRaw.Clone()
	} else {
		transport = http.DefaultTransport
	}

	client := &http.Client{Timeout: 300 * time.Second, Transport: transport}
	start := time.Now()

	fullURL := c.join(c.Host, relativeURL)

	req, err := http.NewRequest(http.MethodGet, fullURL, nil)
	if err != nil {
		return nil, err
	}

	req.Header.Set("User-Agent", "tmtop")

	c.Logger.Debug().Str("url", fullURL).Msg("Doing a query...")

	res, err := client.Do(req)
	if err != nil {
		c.Logger.Warn().Str("url", fullURL).Err(err).Msg("Query failed")
		return nil, err
	}

	c.Logger.Debug().Str("url", fullURL).Dur("duration", time.Since(start)).Msg("Query is finished")

	return res.Body, nil
}

func (c *Client) Get(relativeURL string, target any) error {
	body, err := c.GetInternal(relativeURL)
	if err != nil {
		return err
	}

	if err := json.NewDecoder(body).Decode(target); err != nil {
		return err
	}

	return body.Close()
}

func (c *Client) GetPlain(relativeURL string) ([]byte, error) {
	body, err := c.GetInternal(relativeURL)
	if err != nil {
		return nil, err
	}

	bytes, err := io.ReadAll(body)
	if err != nil {
		return nil, err
	}

	if err := body.Close(); err != nil {
		return nil, err
	}

	return bytes, nil
}
