package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/gin-contrib/cors"
	"github.com/gin-contrib/sessions"
	"github.com/gin-contrib/sessions/cookie"
	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/zkqw3r/FLIBRY/internal/config"
	"github.com/zkqw3r/FLIBRY/internal/database"
	"github.com/zkqw3r/FLIBRY/internal/db"
	"github.com/zkqw3r/FLIBRY/internal/services"
	"github.com/zkqw3r/FLIBRY/internal/torclient"
)

type Handlers struct {
	bookService *services.BookService
	userService *services.UserService
	queries     *db.Queries
	torClient   *http.Client
}

func NewHandlers(bs *services.BookService, us *services.UserService, q *db.Queries, tc *http.Client) *Handlers {
	return &Handlers{
		bookService: bs,
		userService: us,
		queries:     q,
		torClient:   tc,
	}
}

// AuthRequired enforces session authentication
func AuthRequired() gin.HandlerFunc {
	return func(c *gin.Context) {
		session := sessions.Default(c)
		rawUserId := session.Get("user_id")

		if rawUserId == nil {
			if strings.HasPrefix(c.Request.URL.Path, "/api/") {
				c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
			} else {
				c.HTML(http.StatusUnauthorized, "401.html", nil)
			}
			c.Abort()
			return
		}

		var userID int
		switch v := rawUserId.(type) {
		case int:
			userID = v
		case int32:
			userID = int(v)
		case int64:
			userID = int(v)
		case float64:
			userID = int(v)
		default:
			c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid session state"})
			c.Abort()
			return
		}

		c.Set("user_id", userID)
		c.Next()
	}
}

// htmxMiddleware flags HTMX requests
func htmxMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Set("isHTMX", c.GetHeader("HX-Request") == "true")
		c.Next()
	}
}

func (h *Handlers) Index(c *gin.Context) {
	session := sessions.Default(c)
	if session.Get("user_id") == nil {
		c.Redirect(http.StatusFound, "/login")
		return
	}
	c.HTML(http.StatusOK, "index.html", nil)
}

func (h *Handlers) ShowLogin(c *gin.Context)    { c.HTML(http.StatusOK, "login.html", nil) }
func (h *Handlers) ShowRegister(c *gin.Context) { c.HTML(http.StatusOK, "register.html", nil) }

func (h *Handlers) Search(c *gin.Context) {
	query := c.Query("q")
	if query == "" || len(query) < 3 || len(query) > 100 {
		c.HTML(http.StatusOK, "book_card.html", gin.H{"books": []db.Book{}})
		return
	}

	books, err := h.bookService.SearchBooks(c.Request.Context(), query)
	if err != nil {
		c.String(http.StatusInternalServerError, "Search error")
		return
	}

	// Fetch user's saved books to highlight UI buttons accordingly
	userID := c.GetInt("user_id")
	favorites := make(map[int32]bool)
	if userID > 0 {
		userBooks, _ := h.queries.GetUserBooks(c.Request.Context(), pgtype.Int4{Int32: int32(userID), Valid: true})
		for _, b := range userBooks {
			favorites[b.ID] = true
		}
	}

	c.HTML(http.StatusOK, "book_card.html", gin.H{
		"books":     books,
		"favorites": favorites,
	})
}

func (h *Handlers) ToggleLibrary(c *gin.Context) {
	userID := c.GetInt("user_id")
	bookID, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		c.String(http.StatusBadRequest, "Invalid ID")
		return
	}

	ctx := c.Request.Context()
	_, err = h.queries.GetUserBook(ctx, db.GetUserBookParams{
		UserID: pgtype.Int4{Int32: int32(userID), Valid: true},
		BookID: pgtype.Int4{Int32: int32(bookID), Valid: true},
	})

	if err == nil {
		h.queries.RemoveBookFromUser(ctx, db.RemoveBookFromUserParams{
			UserID: pgtype.Int4{Int32: int32(userID), Valid: true},
			BookID: pgtype.Int4{Int32: int32(bookID), Valid: true},
		})
		c.String(http.StatusOK, `<button class="btn btn-secondary" hx-post="/api/library/%d/toggle" hx-swap="outerHTML">В библиотеку</button>`, bookID)
	} else {
		h.queries.AddBookToUser(ctx, db.AddBookToUserParams{
			UserID:     pgtype.Int4{Int32: int32(userID), Valid: true},
			BookID:     pgtype.Int4{Int32: int32(bookID), Valid: true},
			IsFavorite: pgtype.Bool{Bool: true, Valid: true},
		})
		c.String(http.StatusOK, `<button class="btn btn-primary" style="background: #4ade80; border-color: #4ade80; color: #000;" hx-post="/api/library/%d/toggle" hx-swap="outerHTML">✓ Сохранено</button>`, bookID)
	}
}

func (h *Handlers) Reader(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		c.HTML(http.StatusInternalServerError, "reader.html", gin.H{"error": "Invalid book ID"})
		return
	}

	ctx := c.Request.Context()
	book, err := h.queries.GetBookByID(ctx, int32(id))
	if err != nil {
		c.HTML(http.StatusNotFound, "reader.html", gin.H{"error": "Book not found"})
		return
	}

	if !book.FilePath.Valid || book.FilePath.String == "" {
		filePath, err := h.bookService.DownloadBook(ctx, int32(id))
		if err != nil {
			c.HTML(http.StatusInternalServerError, "reader.html", gin.H{"error": "Failed to download the book", "bookID": id})
			return
		}
		book.FilePath = pgtype.Text{String: filePath, Valid: true}
	}

	structure, err := h.bookService.ExtractBookStructure(book.FilePath.String)
	if err != nil {
		c.HTML(http.StatusInternalServerError, "reader.html", gin.H{"error": "Failed to read the book: " + err.Error(), "bookID": id})
		return
	}

	c.HTML(http.StatusOK, "reader.html", gin.H{
		"bookID":    id,
		"book":      book,
		"structure": structure,
	})
}

func (h *Handlers) DownloadFile(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid ID"})
		return
	}

	filePath, err := h.bookService.DownloadBook(c.Request.Context(), int32(id))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to download"})
		return
	}

	var contentType, filename string
	if strings.HasSuffix(filePath, ".fb2") {
		contentType = "application/fb2+zip"
		filename = "book.fb2"
	} else {
		contentType = "application/epub+zip"
		filename = "book.epub"
	}

	c.Header("Content-Type", contentType)
	c.Header("Content-Disposition", fmt.Sprintf("attachment; filename=\"%s\"", filename))
	c.SetCookie("download_started", "1", 10, "/", "", false, false)
	c.File(filePath)
}

func (h *Handlers) DownloadEpubAPI(c *gin.Context) {
	id, _ := strconv.Atoi(c.Param("id"))
	book, _ := h.queries.GetBookByID(c.Request.Context(), int32(id))

	if book.FilePath.Valid && book.FilePath.String != "" {
		c.Header("Content-Type", "application/epub+zip")
		c.File(book.FilePath.String)
		return
	}

	filePath, _ := h.bookService.DownloadBook(c.Request.Context(), int32(id))
	c.Header("Content-Type", "application/epub+zip")
	c.File(filePath)
}

func (h *Handlers) Library(c *gin.Context) {
	userID := c.GetInt("user_id")
	books, _ := h.queries.GetUserBooks(c.Request.Context(), pgtype.Int4{Int32: int32(userID), Valid: true})
	c.HTML(http.StatusOK, "library.html", gin.H{"Books": books})
}

func (h *Handlers) SaveProgress(c *gin.Context) {
	userID := c.GetInt("user_id")
	bookID, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		return
	}

	var p struct {
		Chapter int `json:"chapter"`
		Total   int `json:"total"`
	}

	if err := c.BindJSON(&p); err != nil {
		return
	}

	percent := int32(0)
	if p.Total > 0 {
		percent = int32((float64(p.Chapter) / float64(p.Total)) * 100)
	}

	go h.queries.UpdateReadingProgress(context.Background(), db.UpdateReadingProgressParams{
		ReadingProgress: pgtype.Text{String: strconv.Itoa(p.Chapter), Valid: true},
		ProgressPercent: pgtype.Int4{Int32: percent, Valid: true},
		UserID:          pgtype.Int4{Int32: int32(userID), Valid: true},
		BookID:          pgtype.Int4{Int32: int32(bookID), Valid: true},
	})
	c.Status(http.StatusOK)
}

func (h *Handlers) Profile(c *gin.Context) {
	userID := c.GetInt("user_id")
	user, err := h.queries.GetUserByID(c.Request.Context(), int32(userID))
	if err != nil {
		// Clear session if user is missing from DB
		session := sessions.Default(c)
		session.Clear()
		session.Save()
		c.Redirect(http.StatusFound, "/login")
		return
	}
	c.HTML(http.StatusOK, "profile.html", gin.H{"User": user})
}

func (h *Handlers) RegisterPOST(c *gin.Context) {
	username, email, password := c.PostForm("username"), c.PostForm("email"), c.PostForm("password")
	_, err := h.userService.Register(c.Request.Context(), username, email, password)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "success"})
}

func (h *Handlers) VerifyGET(c *gin.Context) {
	token := c.Query("token")
	err := h.userService.VerifyEmail(c.Request.Context(), token)
	if err != nil {
		c.HTML(http.StatusBadRequest, "401.html", gin.H{"error": "Invalid token"})
		return
	}
	c.Redirect(http.StatusFound, "/login?verified=true")
}

func (h *Handlers) LoginPOST(c *gin.Context) {
	username, password := c.PostForm("username"), c.PostForm("password")
	user, err := h.userService.Login(c.Request.Context(), username, password)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": err.Error()})
		return
	}

	session := sessions.Default(c)
	session.Set("user_id", user.ID)
	session.Save()
	c.JSON(http.StatusOK, gin.H{"message": "success"})
}

func (h *Handlers) LogoutPOST(c *gin.Context) {
	session := sessions.Default(c)
	session.Clear()
	session.Save()
	c.JSON(http.StatusOK, gin.H{"message": "success"})
}

func (h *Handlers) GetCover(c *gin.Context) {
	flibustaID := c.Param("flibustaID")
	placeholderSVG := `<svg xmlns='http://www.w3.org/2000/svg' viewBox='0 0 200 300'><rect width='200' height='300' fill='#181818'/></svg>`

	book, err := h.queries.GetBookByFlibustaID(c.Request.Context(), pgtype.Text{String: flibustaID, Valid: true})
	if err != nil || !book.CoverUrl.Valid || book.CoverUrl.String == "" {
		c.Header("Content-Type", "image/svg+xml")
		c.String(http.StatusOK, placeholderSVG)
		return
	}

	resp, err := h.torClient.Get(book.CoverUrl.String)
	if err != nil {
		c.Header("Content-Type", "image/svg+xml")
		c.String(http.StatusOK, placeholderSVG)
		return
	}
	defer resp.Body.Close()

	c.Header("Content-Type", resp.Header.Get("Content-Type"))
	c.DataFromReader(http.StatusOK, resp.ContentLength, resp.Header.Get("Content-Type"), resp.Body, nil)
}

func (h *Handlers) GetCoverByID(c *gin.Context) {
	id, _ := strconv.Atoi(c.Param("id"))
	book, err := h.queries.GetBookByID(c.Request.Context(), int32(id))
	if err == nil && book.FlibustaID.Valid {
		c.Redirect(http.StatusFound, "/cover/"+book.FlibustaID.String)
		return
	}
	c.String(http.StatusNotFound, "")
}

func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatal(err)
	}
	if err = cfg.Validate(); err != nil {
		log.Fatal(err)
	}

	pool, err := database.Connect(cfg.DBHost, cfg.DBPort, cfg.DBUser, cfg.DBPassword, cfg.DBName, cfg.DBSSLMode)
	if err != nil {
		log.Fatal("Failed to connect to database: ", err)
	}
	defer pool.Close()

	queries := db.New(pool)

	torClient, err := torclient.NewTorClient(cfg.TorProxyURL)
	if err != nil {
		log.Fatal("Cannot connect to Tor proxy")
	}

	emailService, _ := services.NewEmailService("credentials.json")
	bookService := services.NewBookService(queries, torClient, cfg.TorFlibustaURL, cfg.StoragePath)
	userService := services.NewUserService(queries, emailService)
	appHandlers := NewHandlers(bookService, userService, queries, torClient)

	r := gin.Default()

	r.LoadHTMLFiles(
		"../frontentd/templates/index.html",
		"../frontentd/templates/library.html",
		"../frontentd/templates/reader.html",
		"../frontentd/templates/login.html",
		"../frontentd/templates/register.html",
		"../frontentd/templates/partials/book_card.html",
		"../frontentd/templates/layouts/base.html",
		"../frontentd/templates/profile.html",
		"../frontentd/templates/401.html",
	)

	store := cookie.NewStore([]byte(cfg.SessionSecret))
	store.Options(sessions.Options{Path: "/", MaxAge: 86400 * 14, HttpOnly: true})
	r.Use(htmxMiddleware())
	r.Use(sessions.Sessions("flibusta_session", store))
	r.Use(cors.Default())

	r.Static("/static", "../frontentd/static")
	r.StaticFile("/sw.js", "../frontentd/sw.js")
	r.StaticFile("/manifest.json", "../frontentd/manifest.json")

	// Public routes
	r.GET("/", appHandlers.Index)
	r.GET("/login", appHandlers.ShowLogin)
	r.GET("/register", appHandlers.ShowRegister)
	r.GET("/search", appHandlers.Search)
	r.GET("/cover/:flibustaID", appHandlers.GetCover)
	r.GET("/cover-by-id/:id", appHandlers.GetCoverByID)

	r.POST("/register", appHandlers.RegisterPOST)
	r.POST("/login", appHandlers.LoginPOST)
	r.POST("/logout", appHandlers.LogoutPOST)
	r.GET("/verify", appHandlers.VerifyGET)

	// Protected routes
	auth := r.Group("/")
	auth.Use(AuthRequired())
	{
		auth.GET("/library", appHandlers.Library)
		auth.GET("/reader/:id", appHandlers.Reader)
		auth.GET("/profile", appHandlers.Profile)
		auth.GET("/download/:id", appHandlers.DownloadFile)
		auth.GET("/api/books/:id/epub", appHandlers.DownloadEpubAPI)

		auth.POST("/api/library/:id/toggle", appHandlers.ToggleLibrary)
		auth.POST("/api/library/:id/progress", appHandlers.SaveProgress)
	}

	srv := &http.Server{Addr: ":" + cfg.ServerPort, Handler: r}
	go func() {
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatalf("listen: %s\n", err)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	srv.Shutdown(ctx)
}
