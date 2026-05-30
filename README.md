# FLIBRY — Private Flibusta OPDS Reader

<p align="center">
  <img src="https://img.shields.io/badge/Go_1.23+-00ADD8?style=for-the-badge&logo=go&logoColor=white" alt="Go" />
  <img src="https://img.shields.io/badge/Gin-0088CC?style=for-the-badge&logo=go&logoColor=white" alt="Gin" />
  <img src="https://img.shields.io/badge/PostgreSQL_16-336791?style=for-the-badge&logo=postgresql&logoColor=white" alt="PostgreSQL" />
  <img src="https://img.shields.io/badge/Tor_Network-7D4698?style=for-the-badge&logo=tor-project&logoColor=white" alt="Tor" />
  <img src="https://img.shields.io/badge/HTMX-336699?style=for-the-badge&logo=htmx&logoColor=white" alt="HTMX" />
  <img src="https://img.shields.io/badge/Docker-2496ED?style=for-the-badge&logo=docker&logoColor=white" alt="Docker" />
  <img src="https://img.shields.io/badge/PWA-5A0FC8?style=for-the-badge&logo=pwa&logoColor=white" alt="PWA" />
</p>

<div align="center">
  <img src="demonstration.gif" width="60%"></img>
  <p><i>demonstration of work</i></p>
</div>

---

## ✨ Features

- **🧅 Built-in Tor Proxy** — Seamlessly bypasses network restrictions using Tor + Lyrebird (obfs4) to access `.onion` catalogs.
- **📖 Smart Web Reader** — Custom parser extracts chapters and formats text from `.fb2` files for a native reading experience.
- **⚡ HTMX Powered** — SPA-like, blazing-fast UI without the heavy JavaScript payload.
- **💾 Personal Library** — Save favorite books and automatically track your reading progress (current chapter and percentage).
- **🔒 Secure Authentication** — Argon2id password hashing and Gmail OAuth-powered email verification.
- **🛡️ High-Load Ready** — Uses Go's `singleflight` to deduplicate concurrent download requests and atomic file writes to prevent data corruption.
- **📱 Progressive Web App** — Installable on any mobile device or desktop.

---

## 🛠️ Tech Stack

### Backend

| Technology            | Role                                               |
| :-------------------- | :------------------------------------------------- |
| **Go 1.23+**          | Core logic, routing, OPDS parsing, file processing |
| **Gin**               | HTTP router and middleware framework               |
| **sqlc**              | Compile-time type-safe SQL query generation        |
| **pgx/v5**            | High-performance PostgreSQL driver                 |
| **golang.org/x/sync** | `singleflight` for concurrent request deduplication|
| **Gmail API / OAuth** | Sending secure registration verification emails    |

### Frontend

| Technology       | Role                                              |
| :--------------- | :------------------------------------------------ |
| **HTML5 & CSS3** | Custom Glassmorphism UI, mobile-first layout      |
| **HTMX**         | Dynamic, AJAX-driven interactions & DOM swapping  |
| **Vanilla JS**   | Minimal logic for reader controls (font size, UI) |
| **PWA Manifest** | Installable app with Service Worker               |

### Infrastructure

| Technology           | Role                                           |
| :------------------- | :--------------------------------------------- |
| **PostgreSQL 16**    | Relational database for users, books, and progress|
| **Docker & Compose** | Containerized multi-service environment        |
| **Tor + Lyrebird**   | SOCKS5 proxy to route requests to `.onion` URLs|

---

## 📐 Architecture

```text
┌──────────────┐         HTTP / HTMX        ┌──────────────────┐
│              │◄──────────────────────────►│                  │
│   Browser    │                            │   Go Server      │
│   (Client)   │         Media (FB2)        │   (Gin)          │
│              │◄───────────────────────────┤   :8080          │
└──────────────┘                            └──┬─────┬─────┬───┘
                                               │     │     │
             ┌─────────────────────────────────┘     │     │
             │ Google API                            │     │ SQL (sqlc + pgx)
             ▼                                       │     ▼
┌──────────────┐                                     │  ┌──────────────────┐
│              │                                     │  │                  │
│  Gmail API   │                                     │  │   PostgreSQL     │
│ (Auth/Verify)│                                     │  │   :5432          │
│              │                                     │  │                  │
└──────────────┘                                     │  └──────────────────┘
                                                     │
                                                     │ SOCKS5 (x/net/proxy)
                                                     ▼
                                            ┌──────────────────┐
                                            │                  │
                                            │   Tor Proxy      │
                                            │   (obfs4)        │
                                            │   :9050          │
                                            └──┬───────────────┘
                                               │
                                               │ .onion routing
                                               ▼
                                            ┌──────────────────┐
                                            │                  │
                                            │  Flibusta OPDS   │
                                            │                  │
                                            └──────────────────┘
```

> The Go server acts as a bridge. It uses a **Tor SOCKS5 proxy** to securely query the OPDS catalog and download books. Downloaded books are parsed, cached locally (with atomic writes), and served to the client via a clean web reader interface.

---

## 🚀 Install and Run

### Prerequisites

- **Docker** & **Docker Compose**
- Google Cloud Project with OAuth 2.0 Client IDs (for `credentials.json`)

### 1. Clone the repository

```bash
git clone https://github.com/zkqw3r/FLIBRY.git
cd FLIBRY
```

### 2. Configure Environment

1. Copy `.env.example` to `.env` and fill in your database credentials.
2. Place your `credentials.json` (from Google Cloud Console) into the root directory. This is required for the email verification service.

### 3. Start with Docker Compose

```bash
docker-compose up --build -d
```

> **Note:** On the very first run, you may need to check the backend logs (`docker-compose logs backend`) to authorize the Gmail API via a provided link. This will generate a `token.json` file.

### 🌐 Access Points

| Service      | Address               |
| :----------- | :-------------------- |
| **Main App** | http://localhost:8080 |
| **Database** | `localhost:5432`      |

### 🎉 **Open in browser:** The site will be accessible on port 8080 [http://localhost:8080](http://localhost:8080)

---

## 📱 PWA Installation

FLIBRY is built as a Progressive Web App for a native reading experience:

1. Open `localhost:8080` (or your deployed domain) in Chrome / Safari / Edge.
2. Click the **"Install"** prompt (or use the browser menu → "Add to Home Screen").
3. Launch FLIBRY from your home screen — it will open in full-screen mode without browser UI elements.

---

## 📁 Project Structure

```text
FLIBRY/
├── backend/
│   ├── cmd/
│   │   └── app/
│   │       └── main.go              # Application entrypoint & HTTP routes
│   ├── db/
│   │   ├── migrations/              # SQL schemas
│   │   └── queries/                 # SQL queries for sqlc
│   ├── internal/
│   │   ├── config/                  # Environment config loader
│   │   ├── db/                      # Auto-generated sqlc models/methods
│   │   ├── flibusta/                # OPDS XML parser
│   │   ├── services/                # Business logic (Books, Users, Email)
│   │   └── torclient/               # SOCKS5 HTTP client wrapper
│   └── sqlc.yaml                    # sqlc configuration
├── frontend/
│   ├── static/
│   │   └── css/                     # Glassmorphism styles
│   ├── templates/                   # Go HTML templates & HTMX partials
│   ├── manifest.json                # PWA manifest
│   └── sw.js                        # Service worker
├── storage/
│   └── books/                       # Local cache for downloaded FB2/EPUB
├── tor/
│   ├── Dockerfile                   # Alpine image with Tor + obfs4proxy
│   └── torrc                        # Tor configuration with bridges
├── docker-compose.yml               # Multi-container orchestration
└── README.md
```

---

<div align="center">
  <sub>Made with ❤️ by <b>zkqw3r</b></sub>
</div>
