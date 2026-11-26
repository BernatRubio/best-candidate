// Package joboffer loads a job offer from a file or a URL and returns
// it as plain markdown text.
package joboffer

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	htmltomarkdown "github.com/JohannesKaufmann/html-to-markdown/v2"
	"github.com/go-shiori/go-readability"
)

const (
	maxBodyBytes = 5 << 20
	fetchTimeout = 30 * time.Second
)

// Load resolves src to the offer text. An http(s) URL is fetched and reduced
// to its main readable content; anything else is read as a file.
func Load(ctx context.Context, src string) (string, error) {
	var text string
	var err error
	if strings.HasPrefix(src, "http://") || strings.HasPrefix(src, "https://") {
		text, err = fetchURL(ctx, http.DefaultClient, src)
	} else {
		var data []byte
		data, err = os.ReadFile(src)
		text = string(data)
	}
	if err != nil {
		return "", fmt.Errorf("loading job offer: %w", err)
	}
	text = strings.TrimSpace(text)
	if text == "" {
		return "", fmt.Errorf("job offer %q is empty", src)
	}
	return text, nil
}

func readAll(r io.Reader) (string, error) {
	data, err := io.ReadAll(io.LimitReader(r, maxBodyBytes))
	return string(data), err
}

func fetchURL(ctx context.Context, client *http.Client, rawURL string) (string, error) {
	pageURL, err := url.Parse(rawURL)
	if err != nil {
		return "", err
	}
	ctx, cancel := context.WithTimeout(ctx, fetchTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", "best-candidate/0.1")
	req.Header.Set("Accept", "text/html,text/plain;q=0.9,*/*;q=0.8")

	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("GET %s: %s (pages behind a login, such as LinkedIn, cannot be fetched: save the text to a file instead)", rawURL, resp.Status)
	}

	body := io.LimitReader(resp.Body, maxBodyBytes)
	if ct := resp.Header.Get("Content-Type"); strings.HasPrefix(ct, "text/plain") {
		return readAll(body)
	}

	article, err := readability.FromReader(body, pageURL)
	if err != nil {
		return "", fmt.Errorf("extracting readable content: %w", err)
	}
	md, err := htmltomarkdown.ConvertString(article.Content)
	if err != nil {
		return "", fmt.Errorf("converting to markdown: %w", err)
	}
	if article.Title != "" {
		md = "# " + article.Title + "\n\n" + md
	}
	return md, nil
}
