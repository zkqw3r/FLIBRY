package flibusta

import (
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"net/url"
)

// OPDSFeed represents the root element of an OPDS feed
type OPDSFeed struct {
	XMLName xml.Name    `xml:"feed"`
	Entries []OPDSEntry `xml:"entry"`
}

// OPDSEntry represents a single book entry in the OPDS feed
type OPDSEntry struct {
	Title   string       `xml:"title"`
	ID      string       `xml:"id"`
	Authors []OPDSAuthor `xml:"author"`
	Links   []OPDSLink   `xml:"link"`
	Content string       `xml:"content"`
}

// OPDSAuthor represents the author of a book
type OPDSAuthor struct {
	Name string `xml:"name"`
	URI  string `xml:"uri"`
}

// OPDSLink represents a link to download the book or its cover
type OPDSLink struct {
	Rel  string `xml:"rel,attr"`
	Href string `xml:"href,attr"`
	Type string `xml:"type,attr"`
}

func SearchBooks(client *http.Client, baseURL, query string) (*OPDSFeed, error) {
	searchUrl := fmt.Sprintf("%s/opds/search?searchType=books&searchTerm=%s", baseURL, url.QueryEscape(query))

	resp, err := client.Get(searchUrl)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected HTTP status: %d", resp.StatusCode)
	}

	const maxResponseSize = 5 * 1024 * 1024
	limitedReader := io.LimitReader(resp.Body, maxResponseSize)

	body, err := io.ReadAll(limitedReader)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	var feed OPDSFeed
	if err := xml.Unmarshal(body, &feed); err != nil {
		return nil, fmt.Errorf("failed to parse XML: %w", err)
	}

	return &feed, nil
}
