package services

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"log"

	"github.com/alexedwards/argon2id"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/zkqw3r/FLIBRY/internal/db"
)

type UserService struct {
	queries      *db.Queries
	emailService *EmailService
}

func NewUserService(queries *db.Queries, emailService *EmailService) *UserService {
	return &UserService{
		queries:      queries,
		emailService: emailService,
	}
}

// generateSecureToken generates a cryptographically secure random token
func generateSecureToken() (string, error) {
	bytes := make([]byte, 32)
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}
	return hex.EncodeToString(bytes), nil
}

func (s *UserService) Register(ctx context.Context, username, email, password string) (*db.User, error) {
	if len(username) < 3 {
		return nil, fmt.Errorf("username must be at least 3 characters")
	}
	if len(password) < 6 {
		return nil, fmt.Errorf("password must be at least 6 characters")
	}

	_, err := s.queries.GetUserByUsername(ctx, username)
	if err == nil {
		return nil, fmt.Errorf("user already exists")
	}

	hash, err := argon2id.CreateHash(password, argon2id.DefaultParams)
	if err != nil {
		return nil, fmt.Errorf("failed to hash password: %w", err)
	}

	token, err := generateSecureToken()
	if err != nil {
		return nil, fmt.Errorf("failed to generate token: %w", err)
	}

	params := db.CreateUserParams{
		Username:          username,
		Email:             email,
		PasswordHash:      hash,
		VerificationToken: pgtype.Text{String: token, Valid: true},
	}

	user, err := s.queries.CreateUser(ctx, params)
	if err != nil {
		return nil, fmt.Errorf("failed to create user: %w", err)
	}

	// Send verification email asynchronously
	go func() {
		// Graceful degradation if email service is not configured
		if s.emailService == nil {
			log.Printf("[WARNING] Email service is nil. Email to %s not sent.", email)
			return
		}

		err := s.emailService.SendVerificationEmail(email, token)
		if err != nil {
			log.Printf("[ERROR] Failed to send email to %s: %v", email, err)
		} else {
			log.Printf("Verification email sent to %s successfully", email)
		}
	}()

	return &user, nil
}

func (s *UserService) Login(ctx context.Context, username, password string) (*db.User, error) {
	user, err := s.queries.GetUserByUsername(ctx, username)
	if err != nil {
		return nil, fmt.Errorf("invalid credentials")
	}

	if !user.IsVerified.Bool {
		return nil, fmt.Errorf("email is not verified. Please check your inbox")
	}

	match, err := argon2id.ComparePasswordAndHash(password, user.PasswordHash)
	if err != nil {
		return nil, fmt.Errorf("failed to verify password: %w", err)
	}
	if !match {
		return nil, fmt.Errorf("invalid credentials")
	}

	return &user, nil
}

func (s *UserService) VerifyEmail(ctx context.Context, token string) error {
	_, err := s.queries.VerifyUser(ctx, pgtype.Text{String: token, Valid: true})
	if err != nil {
		return fmt.Errorf("invalid or expired token")
	}
	return nil
}
