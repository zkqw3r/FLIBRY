package services

import (
	"archive/zip"
	"context"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/zkqw3r/FLIBRY/internal/db"
	"github.com/zkqw3r/FLIBRY/internal/flibusta"
	"golang.org/x/sync/singleflight"
)

type BookService struct {
	queries     *db.Queries
	torClient   *http.Client
	flibustaUrl string
	storagePath string

	// requestGroup prevents duplicate concurrent downloads for the same book
	requestGroup singleflight.Group
}

func NewBookService(queries *db.Queries, torClient *http.Client, flibustaUrl, storagePath string) *BookService {
	return &BookService{
		queries:     queries,
		torClient:   torClient,
		flibustaUrl: flibustaUrl,
		storagePath: storagePath,
	}
}

func (s *BookService) SearchBooks(ctx context.Context, query string) ([]db.Book, error) {
	// Search in the database first
	params := db.SearchBooksParams{
		Column1: pgtype.Text{String: query, Valid: true},
		Limit:   20,
		Offset:  0,
	}
	data, err := s.queries.SearchBooks(ctx, params)
	if err != nil {
		log.Printf("DB search error: %v", err)
	}

	if len(data) >= 5 {
		log.Printf("Found %d books in DB, returning without Tor", len(data))
		return data, nil
	}

	log.Printf("Found only %d books in DB, trying Tor search...", len(data))

	torCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	feed, err := flibusta.SearchBooks(s.torClient, s.flibustaUrl, query)
	if err != nil {
		log.Printf("Tor search error: %v", err)
		if len(data) > 0 {
			return data, nil
		}
		return []db.Book{}, nil
	}

	results := data
	for _, entry := range feed.Entries {
		select {
		case <-torCtx.Done():
			log.Printf("Tor processing timeout, returning %d books", len(results))
			return results, nil
		default:
		}

		bookId := getId(entry)
		if bookId == "" {
			continue
		}

		existingBook, err := s.queries.GetBookByFlibustaID(ctx, pgtype.Text{String: bookId, Valid: true})
		if err == nil {
			found := false
			for _, r := range results {
				if r.ID == existingBook.ID {
					found = true
					break
				}
			}
			if !found {
				results = append(results, existingBook)
			}
			continue
		}

		coverUrl := getCoverUrl(entry)
		if coverUrl != "" && !strings.HasPrefix(coverUrl, "http") {
			coverUrl = s.flibustaUrl + coverUrl
		}

		createParams := db.CreateBookParams{
			Title:       entry.Title,
			Author:      pgtype.Text{String: getAuthorName(entry), Valid: true},
			FlibustaID:  pgtype.Text{String: bookId, Valid: true},
			CoverUrl:    pgtype.Text{String: coverUrl, Valid: coverUrl != ""},
			Description: pgtype.Text{String: entry.Content, Valid: true},
		}
		book, err := s.queries.CreateBook(ctx, createParams)
		if err != nil {
			log.Printf("Failed to create book %s: %v", entry.Title, err)
			continue
		}
		results = append(results, book)
	}

	return results, nil
}

// DownloadBook downloads a book using singleflight to prevent concurrent duplicate downloads
func (s *BookService) DownloadBook(ctx context.Context, bookID int32) (string, error) {
	book, err := s.queries.GetBookByID(ctx, bookID)
	if err != nil {
		return "", fmt.Errorf("not found: %w", err)
	}

	if book.FilePath.Valid && book.FilePath.String != "" {
		if _, err := os.Stat(book.FilePath.String); err == nil {
			return book.FilePath.String, nil
		}
		log.Printf("File missing from disk, redownloading: %s", book.FilePath.String)
	}

	flibustaID := book.FlibustaID.String
	if !isValidFlibustaID(flibustaID) {
		return "", fmt.Errorf("invalid flibusta_id: contains forbidden characters")
	}

	flightKey := fmt.Sprintf("download_%d", bookID)

	result, err, _ := s.requestGroup.Do(flightKey, func() (interface{}, error) {
		downloadUrl := fmt.Sprintf("%s/b/%s/fb2", s.flibustaUrl, flibustaID)

		req, err := http.NewRequestWithContext(ctx, http.MethodGet, downloadUrl, nil)
		if err != nil {
			return "", err
		}

		resp, err := s.torClient.Do(req)
		fileExt := ".fb2"

		// Fallback to EPUB if FB2 is unavailable
		if err != nil || resp.StatusCode != http.StatusOK {
			if resp != nil {
				resp.Body.Close()
			}
			log.Printf("FB2 not available, trying EPUB for book %s", flibustaID)
			downloadUrl = fmt.Sprintf("%s/b/%s/epub", s.flibustaUrl, flibustaID)

			req, err = http.NewRequestWithContext(ctx, http.MethodGet, downloadUrl, nil)
			if err != nil {
				return "", err
			}
			resp, err = s.torClient.Do(req)
			fileExt = ".epub"
		}

		if err != nil {
			return "", fmt.Errorf("cant download a book: %w", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			return "", fmt.Errorf("failed to download: HTTP %d", resp.StatusCode)
		}

		const maxFileSize = 50 * 1024 * 1024
		limitedReader := io.LimitReader(resp.Body, maxFileSize)
		data, err := io.ReadAll(limitedReader)
		if err != nil {
			return "", fmt.Errorf("failed to read data: %w", err)
		}
		if len(data) >= maxFileSize {
			return "", fmt.Errorf("file too large (>50MB)")
		}

		safeFilename := fmt.Sprintf("%s%s", flibustaID, fileExt)
		path := filepath.Join(s.storagePath, safeFilename)

		if err := os.MkdirAll(s.storagePath, 0755); err != nil {
			return "", fmt.Errorf("failed to create dir: %w", err)
		}

		// Atomic file write: write to temp file first, then rename
		tempPath := path + ".tmp"
		if err := os.WriteFile(tempPath, data, 0644); err != nil {
			return "", fmt.Errorf("failed to save temp file: %w", err)
		}

		if err := os.Rename(tempPath, path); err != nil {
			os.Remove(tempPath)
			return "", fmt.Errorf("failed to rename temp file: %w", err)
		}

		params := db.UpdateBookFilePathParams{
			FilePath: pgtype.Text{String: path, Valid: true},
			ID:       bookID,
		}
		if err := s.queries.UpdateBookFilePath(ctx, params); err != nil {
			return "", fmt.Errorf("failed to update db: %w", err)
		}

		log.Printf("Downloaded book %s to %s", flibustaID, path)
		return path, nil
	})

	if err != nil {
		return "", err
	}

	return result.(string), nil
}

func getId(entry flibusta.OPDSEntry) string {
	for _, link := range entry.Links {
		if link.Type == "application/epub+zip" {
			parts := strings.Split(link.Href, "/")
			if len(parts) > 2 {
				return parts[2]
			}
		}
	}
	return ""
}

func getCoverUrl(entry flibusta.OPDSEntry) string {
	for _, link := range entry.Links {
		if link.Rel == "http://opds-spec.org/image" {
			return link.Href
		}
	}
	return ""
}

func getAuthorName(entry flibusta.OPDSEntry) string {
	if len(entry.Authors) > 0 {
		return entry.Authors[0].Name
	}
	return "Unknown"
}

func isValidFlibustaID(id string) bool {
	for _, ch := range id {
		if !((ch >= '0' && ch <= '9') || (ch >= 'a' && ch <= 'z') || (ch >= 'A' && ch <= 'Z') || ch == '-') {
			return false
		}
	}
	return len(id) > 0 && len(id) < 100
}

type Chapter struct {
	Index   int    `json:"index"`
	Title   string `json:"title"`
	Content string `json:"content"`
}

type BookStructure struct {
	Chapters []Chapter `json:"chapters"`
	Title    string    `json:"title"`
}

func (s *BookService) ExtractTextFromEpub(filePath string) (string, error) {
	structure, err := s.ExtractBookStructure(filePath)
	if err != nil {
		return "", err
	}

	var textContent strings.Builder
	for _, chapter := range structure.Chapters {
		if chapter.Title != "" {
			textContent.WriteString("=== " + chapter.Title + " ===\n\n")
		}
		textContent.WriteString(chapter.Content)
		textContent.WriteString("\n\n")
	}

	return textContent.String(), nil
}

func (s *BookService) ExtractBookStructure(filePath string) (*BookStructure, error) {
	ext := strings.ToLower(filepath.Ext(filePath))
	if ext == ".fb2" {
		return s.extractFB2Structure(filePath)
	}
	return s.extractEPUBStructure(filePath)
}

func (s *BookService) extractFB2Structure(filePath string) (*BookStructure, error) {
	var content []byte
	var err error

	if strings.HasSuffix(filePath, ".zip") || strings.HasSuffix(filePath, ".fb2") {
		r, zipErr := zip.OpenReader(filePath)
		if zipErr == nil {
			defer r.Close()
			for _, f := range r.File {
				if strings.HasSuffix(strings.ToLower(f.Name), ".fb2") {
					rc, err := f.Open()
					if err != nil {
						return nil, fmt.Errorf("failed to open fb2 in zip: %w", err)
					}
					content, err = io.ReadAll(rc)
					rc.Close()
					if err != nil {
						return nil, fmt.Errorf("failed to read fb2 from zip: %w", err)
					}
					break
				}
			}
			if len(content) == 0 {
				return nil, fmt.Errorf("no fb2 file found in zip archive")
			}
		} else {
			content, err = os.ReadFile(filePath)
			if err != nil {
				return nil, fmt.Errorf("failed to read fb2: %w", err)
			}
		}
	} else {
		content, err = os.ReadFile(filePath)
		if err != nil {
			return nil, fmt.Errorf("failed to read fb2: %w", err)
		}
	}

	xmlContent := string(content)
	structure := &BookStructure{
		Chapters: []Chapter{},
	}

	htmlTagRegex := regexp.MustCompile(`<[^>]*>`)
	sectionRegex := regexp.MustCompile(`(?s)<section[^>]*>(.*?)</section>`)
	sections := sectionRegex.FindAllStringSubmatch(xmlContent, -1)

	if len(sections) == 0 {
		plainText := htmlTagRegex.ReplaceAllString(xmlContent, " ")
		plainText = strings.Join(strings.Fields(plainText), " ")
		structure.Chapters = append(structure.Chapters, Chapter{Index: 1, Title: "Book", Content: plainText})
		return structure, nil
	}

	chapterNum := 0
	for _, section := range sections {
		sectionContent := section[1]
		titleRegex := regexp.MustCompile(`(?s)<title[^>]*>(.*?)</title>`)
		titleMatch := titleRegex.FindStringSubmatch(sectionContent)
		title := ""
		if len(titleMatch) > 1 {
			title = htmlTagRegex.ReplaceAllString(titleMatch[1], " ")
			title = strings.TrimSpace(title)
			title = strings.Join(strings.Fields(title), " ")
		}

		textContent := titleRegex.ReplaceAllString(sectionContent, "")
		plainText := htmlTagRegex.ReplaceAllString(textContent, " ")
		plainText = strings.Join(strings.Fields(plainText), " ")
		plainText = strings.TrimSpace(plainText)

		if len(plainText) < 100 {
			continue
		}
		chapterNum++
		if title == "" {
			title = fmt.Sprintf("Chapter %d", chapterNum)
		}
		if len(title) > 100 {
			title = title[:100] + "..."
		}
		structure.Chapters = append(structure.Chapters, Chapter{Index: chapterNum, Title: title, Content: plainText})
	}

	if len(structure.Chapters) == 0 {
		return nil, fmt.Errorf("no chapters found in fb2")
	}
	return structure, nil
}

func (s *BookService) extractEPUBStructure(filePath string) (*BookStructure, error) {
	r, err := zip.OpenReader(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to open epub: %w", err)
	}
	defer r.Close()

	structure := &BookStructure{
		Chapters: []Chapter{},
	}

	htmlTagRegex := regexp.MustCompile(`<[^>]*>`)
	type fileWithName struct {
		file *zip.File
		name string
	}
	var contentFiles []fileWithName

	for _, f := range r.File {
		if strings.HasSuffix(f.Name, ".html") || strings.HasSuffix(f.Name, ".xhtml") || strings.HasSuffix(f.Name, ".htm") {
			lowerName := strings.ToLower(f.Name)
			if strings.Contains(lowerName, "nav.xhtml") || strings.Contains(lowerName, "toc.") || strings.Contains(lowerName, "cover") || strings.Contains(lowerName, "title") {
				continue
			}
			contentFiles = append(contentFiles, fileWithName{file: f, name: f.Name})
		}
	}

	if len(contentFiles) == 0 {
		return nil, fmt.Errorf("no content files found in epub")
	}

	sort.Slice(contentFiles, func(i, j int) bool {
		return contentFiles[i].name < contentFiles[j].name
	})

	var fullHTML strings.Builder
	for _, fw := range contentFiles {
		rc, err := fw.file.Open()
		if err != nil {
			log.Printf("Failed to open file %s: %v", fw.name, err)
			continue
		}
		content, err := io.ReadAll(rc)
		rc.Close()
		if err != nil {
			log.Printf("Failed to read file %s: %v", fw.name, err)
			continue
		}
		htmlContent := string(content)
		fullHTML.WriteString(htmlContent)
		fullHTML.WriteString("\n")
	}

	htmlContent := fullHTML.String()
	chapters := extractChaptersByPatterns(htmlContent)

	if len(chapters) == 0 {
		plainText := htmlTagRegex.ReplaceAllString(htmlContent, " ")
		plainText = strings.Join(strings.Fields(plainText), " ")
		structure.Chapters = append(structure.Chapters, Chapter{Index: 1, Title: "Book", Content: plainText})
	} else {
		structure.Chapters = chapters
	}

	return structure, nil
}

func extractChaptersByPatterns(htmlContent string) []Chapter {
	var chapters []Chapter
	htmlTagRegex := regexp.MustCompile(`<[^>]*>`)

	patterns := []struct {
		name  string
		regex *regexp.Regexp
	}{
		{"roman_header", regexp.MustCompile(`(?i)<h[1-3][^>]*>\s*(I|II|III|IV|V|VI|VII|VIII|IX|X|XI|XII|XIII|XIV|XV|XVI|XVII|XVIII|XIX|XX|XXI|XXII|XXIII|XXIV|XXV|XXVI|XXVII|XXVIII|XXIX|XXX)\s*</h[1-3]>`)},
		{"chapter_header", regexp.MustCompile(`(?i)<h[1-3][^>]*>\s*(Глава|Часть|Раздел|Chapter|Part)\s+(\d+|[IVXLCDM]+)\s*[:\.]?\s*(.*?)</h[1-3]>`)},
		{"number_header", regexp.MustCompile(`(?i)<h[1-3][^>]*>\s*(\d+)\s*</h[1-3]>`)},
		{"any_header", regexp.MustCompile(`(?i)<h[1-2][^>]*>(.*?)</h[1-2]>`)},
	}

	for _, pattern := range patterns {
		matches := pattern.regex.FindAllStringSubmatchIndex(htmlContent, -1)
		if len(matches) < 2 {
			continue
		}

		for i, match := range matches {
			titleStart := match[0]
			titleEnd := match[1]
			titleHTML := htmlContent[titleStart:titleEnd]
			title := htmlTagRegex.ReplaceAllString(titleHTML, "")
			title = strings.TrimSpace(title)

			contentStart := titleEnd
			contentEnd := len(htmlContent)
			if i < len(matches)-1 {
				contentEnd = matches[i+1][0]
			}

			chapterHTML := htmlContent[contentStart:contentEnd]
			chapterText := htmlTagRegex.ReplaceAllString(chapterHTML, " ")
			chapterText = strings.Join(strings.Fields(chapterText), " ")
			chapterText = strings.TrimSpace(chapterText)

			if len(chapterText) < 100 {
				continue
			}
			if len(title) > 100 {
				title = title[:100] + "..."
			}
			if len(title) < 1 {
				title = fmt.Sprintf("Chapter %d", i+1)
			}
			chapters = append(chapters, Chapter{Index: i + 1, Title: title, Content: chapterText})
		}
		if len(chapters) > 0 {
			return chapters
		}
	}
	return chapters
}

func (s *BookService) GetChapter(filePath string, chapterIndex int) (*Chapter, error) {
	structure, err := s.ExtractBookStructure(filePath)
	if err != nil {
		return nil, err
	}
	if chapterIndex < 1 || chapterIndex > len(structure.Chapters) {
		return nil, fmt.Errorf("chapter index out of range")
	}
	return &structure.Chapters[chapterIndex-1], nil
}
