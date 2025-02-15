package jwtutil

import (
	"crypto/rsa"
	"errors"
	"grpc-benchmark-study/internal/resources"
	"time"

	"github.com/golang-jwt/jwt/v4"
)

// Global variables holding the parsed keys.
var (
	rsaPrivateKey *rsa.PrivateKey
	rsaPublicKey  *rsa.PublicKey
)

// LoadKeys loads the RSA private and public keys from the given file paths.
// The private key is used for signing tokens and the public key for validating them.
func LoadKeys(privatePath, publicPath string) error {
	// Load the private key.

	//privBytes, err := os.ReadFile(privatePath)
	privBytes, err := resources.JWT.ReadFile(privatePath)
	if err != nil {
		return err
	}
	privKey, err := jwt.ParseRSAPrivateKeyFromPEM(privBytes)
	if err != nil {
		return err
	}
	rsaPrivateKey = privKey

	// Load the public key.
	pubBytes, err := resources.JWT.ReadFile(publicPath)
	if err != nil {
		return err
	}
	pubKey, err := jwt.ParseRSAPublicKeyFromPEM(pubBytes)
	if err != nil {
		return err
	}
	rsaPublicKey = pubKey

	return nil
}

// GenerateToken creates a JWT token with the provided claims.
// If the "exp" (expiration) claim is not set, it defaults to 1 hour from now.
// The token is signed using the RSA private key (loaded via LoadKeys).
func GenerateToken(claims jwt.MapClaims) (string, error) {
	if rsaPrivateKey == nil {
		return "", errors.New("private key not loaded")
	}

	// Set default expiration and issued-at if not provided.
	if _, ok := claims["exp"]; !ok {
		claims["exp"] = time.Now().Add(time.Hour).Unix()
	}
	if _, ok := claims["iat"]; !ok {
		claims["iat"] = time.Now().Unix()
	}

	// Create a new token with the provided claims.
	token := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)

	// Sign the token using the RSA private key.
	tokenString, err := token.SignedString(rsaPrivateKey)
	if err != nil {
		return "", err
	}
	return tokenString, nil
}

// ValidateToken parses and validates the given token string using the RSA public key.
// If successful, it returns the parsed token; otherwise, an error is returned.
func ValidateToken(tokenString string) (*jwt.Token, error) {
	if rsaPublicKey == nil {
		return nil, errors.New("public key not loaded")
	}

	token, err := jwt.Parse(tokenString, func(token *jwt.Token) (interface{}, error) {
		// Ensure the signing algorithm is RS256.
		if _, ok := token.Method.(*jwt.SigningMethodRSA); !ok {
			return nil, errors.New("unexpected signing method")
		}
		return rsaPublicKey, nil
	})
	if err != nil {
		return nil, err
	}
	if !token.Valid {
		return nil, errors.New("invalid token")
	}
	return token, nil
}
