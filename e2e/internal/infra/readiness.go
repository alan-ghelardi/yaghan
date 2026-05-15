package infra

import (
	"context"
	"fmt"
	"net"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/health/grpc_health_v1"
)

// poll re-invokes probe every interval until it succeeds or ctx is
// done. Returns the last error if the deadline trips before probe
// returns nil.
func poll(ctx context.Context, interval time.Duration, probe func(context.Context) error) error {
	var lastErr error
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	// Try once up-front so a service that's already healthy doesn't
	// wait the first interval.
	if err := probe(ctx); err == nil {
		return nil
	} else {
		lastErr = err
	}

	for {
		select {
		case <-ctx.Done():
			return fmt.Errorf("readiness: %w (last probe error: %v)", ctx.Err(), lastErr)
		case <-ticker.C:
			if err := probe(ctx); err == nil {
				return nil
			} else {
				lastErr = err
			}
		}
	}
}

// WaitForDynamoDB polls ListTables until it succeeds. DynamoDB Local
// has no in-image healthcheck (the upstream image lacks curl/wget/nc),
// so a successful API call is the readiness signal — same approach as
// api-server/dev/start.sh.
func WaitForDynamoDB(ctx context.Context, client *dynamodb.Client) error {
	return poll(ctx, 500*time.Millisecond, func(ctx context.Context) error {
		_, err := client.ListTables(ctx, &dynamodb.ListTablesInput{})
		return err
	})
}

// WaitForRedis polls a Redis address with a RESP `PING` until it gets
// back `+PONG`. Avoids pulling in the redis client just for readiness.
func WaitForRedis(ctx context.Context, addr string) error {
	return poll(ctx, 500*time.Millisecond, func(ctx context.Context) error {
		d := net.Dialer{Timeout: 2 * time.Second}
		conn, err := d.DialContext(ctx, "tcp", addr)
		if err != nil {
			return err
		}
		defer func() { _ = conn.Close() }()
		if _, err := conn.Write([]byte("PING\r\n")); err != nil {
			return err
		}
		buf := make([]byte, 7)
		if _, err := conn.Read(buf); err != nil {
			return err
		}
		if string(buf[:5]) != "+PONG" {
			return fmt.Errorf("redis: unexpected reply %q", buf)
		}
		return nil
	})
}

// WaitForS3 polls ListBuckets until it succeeds — same pattern
// daemon/dev/start.sh uses against MinIO.
func WaitForS3(ctx context.Context, client *s3.Client) error {
	return poll(ctx, 500*time.Millisecond, func(ctx context.Context) error {
		_, err := client.ListBuckets(ctx, &s3.ListBucketsInput{})
		return err
	})
}

// WaitForGRPCHealth polls a server's standard grpc.health.v1.Health
// service (registered by every server built on commons/pkg/server) and
// returns nil when it reports SERVING. The empty service name asks for
// overall server health.
func WaitForGRPCHealth(ctx context.Context, addr string) error {
	conn, err := grpc.NewClient(addr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return fmt.Errorf("dial %s: %w", addr, err)
	}
	defer func() { _ = conn.Close() }()
	client := grpc_health_v1.NewHealthClient(conn)

	return poll(ctx, 500*time.Millisecond, func(ctx context.Context) error {
		resp, err := client.Check(ctx, &grpc_health_v1.HealthCheckRequest{})
		if err != nil {
			return err
		}
		if resp.GetStatus() != grpc_health_v1.HealthCheckResponse_SERVING {
			return fmt.Errorf("health: status=%s", resp.GetStatus())
		}
		return nil
	})
}
