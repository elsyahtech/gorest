# Redis

GOREST integrates Redis as an application dependency for caching and other
Redis-backed operations. Configure Redis through the Redis package and
register it with the application when needed. Redis initialization is enabled
by default; set `Disable: true` or omit Redis registration when the application
does not use Redis.

## Operations and lifecycle

GOREST exposes the configured Redis client for Redis commands and connection
pool operations. Its automated integration coverage includes Redis operations,
concurrent use, timeouts and cancellation, startup failure, cleanup, shutdown,
and reconnecting clients after existing client sockets are dropped.

Client reconnection does not by itself establish Sentinel/Cluster failover,
persistence, backup/restore, or recovery after every outage. Validate those
requirements against the topology used by your application.

## TLS

Configure TLS explicitly for Redis network connections. GOREST verifies
server certificates when TLS is enabled and does not enable
`InsecureSkipVerify`. Set `SSLCAPath` when using a custom CA. When mTLS is
required, configure both `SSLCertPath` and `SSLKeyPath`, and require trusted
client certificates on the Redis service.

Avoid disabling certificate verification. A localhost TLS setup is useful
for exercising certificate loading and handshakes; for deployment, verify
that the service hostname matches the certificate and that the issuing CA is
trusted.

See the [Redis configuration guide](../configuration.md) and
[integration test guide](../../integration_test/README.md) for settings and
test environment variables. Keep credentials and private keys in protected
configuration, not source control.
