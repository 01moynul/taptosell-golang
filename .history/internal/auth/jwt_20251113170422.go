// ValidateToken parses and validates a JWT token string.
// It returns the user ID (subject) if the token is valid.
func ValidateToken(tokenString string) (int64, error) {
	// 1. Parse the token string.
	token, err := jwt.Parse(tokenString, func(token *jwt.Token) (interface{}, error) {
		// 2. Check the signing method.
		// This ensures the token was signed with the same algorithm we use.
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, errors.New("unexpected signing method")
		}

		// 3. Return our secret key for validation.
		return jwtSecretKey, nil
	})
	if err != nil {
		return 0, err // Token parsing failed (e.g., expired, malformed)
	}

	// 4. Check if the token is valid and get the claims.
	if claims, ok := token.Claims.(jwt.MapClaims); ok && token.Valid {
		// 5. Get the user ID ("sub") from the claims.
		userIDFloat, ok := claims["sub"].(float64)
		if !ok {
			return 0, errors.New("invalid subject claim")
		}
		// Convert the float64 (JSON's number type) to int64
		return int64(userIDFloat), nil
	}

	return 0, errors.New("invalid token")
}