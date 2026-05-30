CREATE TABLE books (
    id SERIAL PRIMARY KEY,
    title VARCHAR(500) NOT NULL,
    author VARCHAR(500),
    flibusta_id VARCHAR(100) UNIQUE,
    file_path VARCHAR(1000),
    cover_url VARCHAR(1000),
    description TEXT,
    created_at TIMESTAMP DEFAULT NOW()
);

CREATE TABLE users (
    id SERIAL PRIMARY KEY,
    username VARCHAR(100) UNIQUE NOT NULL,
    email VARCHAR(255) UNIQUE NOT NULL,
    password_hash VARCHAR(255) NOT NULL,
    is_verified BOOLEAN DEFAULT FALSE,
    verification_token VARCHAR(255),
    created_at TIMESTAMP DEFAULT NOW()
);

CREATE TABLE user_books (
    id SERIAL PRIMARY KEY,
    user_id INTEGER REFERENCES users(id) ON DELETE CASCADE,
    book_id INTEGER REFERENCES books(id) ON DELETE CASCADE,
    is_favorite BOOLEAN DEFAULT FALSE,
    reading_progress VARCHAR(500),  -- CFI позиция в книге
    progress_percent INTEGER DEFAULT 0,
    last_read_at TIMESTAMP,
    UNIQUE(user_id, book_id)
);