package auth

import (
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/alexedwards/argon2id"
	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
)

func HashPassword(pass string) (string, error) {
	params := argon2id.Params{
		Memory:      64 * 1024,
		Iterations:  3,
		Parallelism: 1,
		SaltLength:  12,
		KeyLength:   32,
	}

	hash, err := argon2id.CreateHash(pass, &params)
	if err != nil {
		return "", err
	}
	return hash, nil
}

func CheckPasswordHash(pass, hash string) (bool, error) {
	return argon2id.ComparePasswordAndHash(pass, hash)
}

func MakeJWT(userID uuid.UUID, tokenSecret string, expiresIn time.Duration) (string, error) {
	now := time.Now()
	expires := now.Add(expiresIn)

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.RegisteredClaims{
		Issuer:    "testdb",
		IssuedAt:  jwt.NewNumericDate(now),
		ExpiresAt: jwt.NewNumericDate(expires),
		Subject:   userID.String(),
	})
	signingKey := []byte(tokenSecret)
	return token.SignedString(signingKey)
}

// Validate returns the id associated with the JWT - it is still up to the user to validate the id
func ValidateJWT(tokenString, tokenSecret string) (uuid.UUID, error) {
	claims := &jwt.RegisteredClaims{}

	token, err := jwt.ParseWithClaims(tokenString, claims, func(token *jwt.Token) (interface{}, error) {
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("Unexpected signing method: %v", token.Header["alg"])
		}
		return []byte(tokenSecret), nil
	})
	if err != nil {
		return uuid.Nil, fmt.Errorf("Token validation failed: %v", err)
	}

	if !token.Valid {
		return uuid.Nil, fmt.Errorf("Token is invalid")
	}

	id, err := uuid.Parse(claims.Subject)
	if err != nil {
		return uuid.Nil, fmt.Errorf("Could not parse subject into a uuid: %s", claims.Subject)
	}

	return id, nil
}

func GetBearerToken(headers http.Header) (string, error) {
	authValues := headers["Authorization"]
	if len(authValues) == 0 {
		return "", fmt.Errorf("No values in the authorization header")
	}
	parts := strings.Fields(authValues[0])
	if len(parts) != 2 || parts[0] != "Bearer" {
		return "", fmt.Errorf("Missing or imporperly formatted header token: %s", authValues[0])
	}
	token := parts[1]
	return token, nil
}
