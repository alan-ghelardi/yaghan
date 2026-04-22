package testing

import (
	"crypto/rand"
	"crypto/rsa"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/lestrrat-go/jwx/v3/jwa"
	"github.com/lestrrat-go/jwx/v3/jwk"
	"github.com/lestrrat-go/jwx/v3/jwt"
	"golang.nuinfra.net/commons/pkg/utilities"
)

type Claims map[string]any

// SignedJWTTokenAndKeySet generates a signed JWT token and returns it along with
// the public key required to verify the token's validity. Custom claims can
// be set using the provided `claims` argument. The generated token expires
// in five minutes.
func SignedJWTTokenAndKeySet(t *testing.T, claims Claims) (string, jwk.Set) {
	t.Helper()

	rawKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("unable to generate RSA key: %v", err)
	}
	key, err := jwk.Import(rawKey)
	if err != nil {
		t.Fatalf("unable to import RSA key: %v", err)
	}

	if err := key.Set("alg", jwa.RS256()); err != nil {
		t.Fatal(err)
	}

	tokenBuilder := jwt.NewBuilder().
		Issuer("auth.example.dev").
		IssuedAt(time.Now()).
		Expiration(time.Now().Add(5 * time.Minute))

	for name, value := range claims {
		tokenBuilder = tokenBuilder.Claim(name, value)
	}

	token, err := tokenBuilder.Build()
	if err != nil {
		t.Fatalf("unable to create JWT token: %v", err)
	}

	signedToken, err := jwt.Sign(token, jwt.WithKey(jwa.RS256(), key))
	if err != nil {
		t.Fatalf("unable to sign JWT token: %v", err)
	}

	publicKey, err := jwk.PublicKeyOf(key)
	if err != nil {
		t.Fatalf("unable to extract JWK public key: %v", err)
	}

	keySet := jwk.NewSet()
	if err := keySet.AddKey(publicKey); err != nil {
		t.Fatal(err)
	}

	return string(signedToken), keySet
}

func WriteKey(t *testing.T, keySet jwk.Set) string {
	t.Helper()

	keyData, err := json.Marshal(keySet)
	if err != nil {
		t.Fatalf("unable to marshal JWK public key: %v", err)
	}

	keyFile := filepath.Join(os.TempDir(), utilities.RandomString(7))

	if err := os.WriteFile(keyFile, keyData, 0600); err != nil {
		t.Fatalf("unable to write key to %s: %v", keyFile, err)
	}

	return keyFile
}
