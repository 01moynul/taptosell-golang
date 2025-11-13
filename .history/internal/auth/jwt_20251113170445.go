package auth

import (
	"errors"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// We will hardcode our secret key for now.
// In a real production app, we would load this from a secure .env file.
// This key is used to "sign" our passports so we know they are real.
var jwtSecretKey = []byte("A_VERY_SECURE_SECRET_KEY_REPLACE_LATER")

// GenerateToken creates a new JWT (passport) for a given user ID.
func GenerateToken(userID int64) (string, error) {
	// 1. Create the "claims" (the data inside the passport).
	// We are claiming that this token is for a specific 'userID'.
	// We also set an expiration time (72 hours).
	claims := jwt.MapClaims{
		"sub": userID,                                // "sub" (Subject) is the standard claim for User ID
		"exp": time.Now().Add(time.Hour * 72).Unix(), // Expires in 3 days
		"iat": time.Now().Unix(),                     // "iat" (Issued At)
	}

	// 2. Create the token object
	// We sign it using the 'HS256' algorithm and our claims.
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)

	// 3. Sign the token with our secret key
	// This creates the final, secure token string.
	tokenString, err := token.SignedString(jwtSecretKey)
	if err != nil {
		return "", err
	}

	return tokenString, nil
}

// ValidateToken (we will build this later)
func ValidateToken(tokenString string) (int64, error) {
	// This function will be used for our protected routes
	return 0, errors.New("validate function not implemented")
}
