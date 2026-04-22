package testing

import (
	"testing"

	"github.com/testcontainers/testcontainers-go/modules/redis"
)

// WithRedis starts a Redis container for the duration of the test and returns its endpoint.
func WithRedis(t *testing.T) string {
	t.Helper()

	redisContainer, err := redis.Run(t.Context(), "redis")
	if err != nil {
		t.Fatal(err)
	}

	t.Cleanup(func() {
		err := redisContainer.Terminate(t.Context())
		if err != nil {
			t.Logf("warning: unable to terminate Redis container: %v", err)
		}
	})

	endpoint, err := redisContainer.Endpoint(t.Context(), "")
	if err != nil {
		t.Fatal(err)
	}

	return endpoint
}
