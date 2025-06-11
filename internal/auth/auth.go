package auth

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"
)

func HashPassword(password string) (string, error) {
	hash, err := bcrypt.GenerateFromPassword([]byte(password), 10)
	if err != nil {
		return "", err
	}
	return string(hash), err
}
func CheckPasswordHash(hash, password string) error {
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(password))
}
func MakeJWT(userID uuid.UUID, tokenSecret string) (string, error) {
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.RegisteredClaims{
		Issuer: "chirpy",
		IssuedAt: &jwt.NumericDate{
			Time: time.Now(),
		},
		ExpiresAt: &jwt.NumericDate{
			Time: time.Now().Add(time.Hour),
		},
		Subject: userID.String(),
	})
	signedJWT, err := token.SignedString([]byte(tokenSecret))
	if err != nil {
		return "", err
	}
	return signedJWT, nil
}
func ValidateJWT(tokenString, tokenSecret string) (uuid.UUID, error) {
	token, err := jwt.ParseWithClaims(tokenString, &jwt.RegisteredClaims{}, func(t *jwt.Token) (interface{}, error) {
		return []byte(tokenSecret), nil
	})
	if err != nil {
		return uuid.UUID{}, err
	}
	val, err := token.Claims.GetSubject()
	if err != nil {
		return uuid.UUID{}, err
	}
	userID, err := uuid.Parse(val)
	if err != nil {
		return uuid.UUID{}, err
	}
	return userID, err
}
func GetBearerToken(headers http.Header) (string, error) {
	token := headers.Get("Authorization")
	if token == "" {
		return "", fmt.Errorf("Error Authorization header was empty")
	}
	if !strings.HasPrefix(token, "Bearer ") {
		return "", fmt.Errorf("Error Authorization header did not contain prefix Bearer")
	}
	token = strings.TrimPrefix(token, "Bearer ")
	token = strings.TrimSpace(token)
	if token == "" {
		return "", fmt.Errorf("Error Authorization header did not contain a token")
	}
	return token, nil
}
func MakeRefreshTokenString() (string, error) {
	tokenInBytes := make([]byte, 32)
	rand.Read(tokenInBytes)
	token := hex.EncodeToString(tokenInBytes)
	return token, nil
}
func GetApiKey(headers http.Header) (string, error) {
	apiKey := headers.Get("Authorization")
	if apiKey == "" {
		return "", fmt.Errorf("Error Authorization header was empty")
	}
	if !strings.HasPrefix(apiKey, "ApiKey ") {
		return "", fmt.Errorf("Error Authorization header did not contain prefix Api Key")
	}
	apiKey = strings.TrimPrefix(apiKey, "ApiKey ")
	apiKey = strings.TrimSpace(apiKey)
	if apiKey == "" {
		return "", fmt.Errorf("Error Authorization header did not contain a Api Key")
	}
	return apiKey, nil
}
