package services

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
	"google.golang.org/api/gmail/v1"
	"google.golang.org/api/option"
)

type EmailService struct {
	gmailService *gmail.Service
}

func getTokenFromWeb(config *oauth2.Config) (*oauth2.Token, error) {
	url := config.AuthCodeURL("state-token", oauth2.AccessTypeOffline)
	fmt.Printf("Please authorize gmail OAuth2.0:\n%s\n", url)
	fmt.Print("Enter verification code: ")
	var authCode string
	if _, err := fmt.Scan(&authCode); err != nil {
		return nil, fmt.Errorf("unable to read authorization code: %w", err)
	}

	tok, err := config.Exchange(context.Background(), authCode)
	if err != nil {
		return nil, fmt.Errorf("unable to retrieve token: %w", err)
	}
	return tok, saveToken("token.json", tok)
}

func saveToken(path string, token *oauth2.Token) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()

	return json.NewEncoder(f).Encode(token)
}

func getToken(config *oauth2.Config) (*oauth2.Token, error) {
	tokFile := "token.json"
	file, err := os.Open(tokFile)
	if err != nil {
		return getTokenFromWeb(config)
	}
	defer file.Close()

	tok := &oauth2.Token{}
	err = json.NewDecoder(file).Decode(tok)
	if err != nil {
		return nil, fmt.Errorf("unable to decode token: %w", err)
	}
	return tok, nil
}

func NewEmailService(credentialsPath string) (*EmailService, error) {
	b, err := os.ReadFile(credentialsPath)
	if err != nil {
		return nil, fmt.Errorf("unable to read credentials: %w", err)
	}

	config, err := google.ConfigFromJSON(b, gmail.GmailSendScope)
	if err != nil {
		return nil, fmt.Errorf("unable to parse credentials: %w", err)
	}

	token, err := getToken(config)
	if err != nil {
		return nil, fmt.Errorf("unable to get token config: %w", err)
	}

	srv, err := gmail.NewService(context.Background(), option.WithTokenSource(config.TokenSource(context.Background(), token)))
	if err != nil {
		return nil, fmt.Errorf("unable to create gmail service: %w", err)
	}

	return &EmailService{
		gmailService: srv,
	}, nil
}

func (s *EmailService) SendVerificationEmail(to, token string) error {
	verificationLink := fmt.Sprintf("http://localhost:8080/verify?token=%s", token)
	subject := "FLIBRY Registration Confirmation"
	body := fmt.Sprintf("Welcome!\n\nPlease follow the link to confirm your registration:\n%s\n", verificationLink)

	var message strings.Builder
	message.WriteString(fmt.Sprintf("To: %s\r\n", to))
	message.WriteString("Subject: " + subject + "\r\n")
	message.WriteString("MIME-Version: 1.0\r\n")
	message.WriteString("Content-Type: text/plain; charset=utf-8\r\n")
	message.WriteString("\r\n")
	message.WriteString(body)

	// Use URLEncoding without padding to comply with Google API requirements (avoids 400 Invalid Base64 error).
	raw := base64.URLEncoding.WithPadding(base64.NoPadding).EncodeToString([]byte(message.String()))

	gmailMessage := &gmail.Message{
		Raw: raw,
	}

	_, err := s.gmailService.Users.Messages.Send("me", gmailMessage).Do()
	if err != nil {
		return fmt.Errorf("unable to send email: %w", err)
	}
	return nil
}
