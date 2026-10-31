//go:build integration

package mongo_test

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/elsyahtech/gorest/database"
	"go.mongodb.org/mongo-driver/v2/bson"
)

func TestMongoMTLSExternal(t *testing.T) {
	const prefix = "GOREST_IT_MONGO_MTLS"
	if strings.TrimSpace(os.Getenv(prefix+"_HOST")) == "" {
		t.Skipf("set %s_HOST to run the external MongoDB mTLS test", prefix)
	}

	config := database.ConfigFromEnv(prefix)
	config.Driver = database.MONGO
	config.Timeout = 10 * time.Second
	for _, name := range []string{
		"PORT", "NAME", "USERNAME", "PASSWORD", "SSL_CA", "SSL_CERT", "SSL_KEY",
	} {
		if strings.TrimSpace(os.Getenv(prefix+"_"+name)) == "" {
			t.Fatalf("required external MongoDB mTLS environment variable %s_%s is not set", prefix, name)
		}
	}
	if config.TLS != "true" {
		t.Fatalf("%s_TLS must be true for MongoDB mTLS tests", prefix)
	}

	withoutCertificate := config
	withoutCertificate.SSLCertPath = ""
	withoutCertificate.SSLKeyPath = ""
	noCertCtx, cancelNoCert := context.WithTimeout(context.Background(), 12*time.Second)
	withoutCertDB, _, err := database.New(noCertCtx, &withoutCertificate, "UTC")
	cancelNoCert()
	if err == nil {
		disconnectCtx, cancelDisconnect := context.WithTimeout(context.Background(), 5*time.Second)
		if disconnectErr := withoutCertDB.Mongo.Disconnect(disconnectCtx); disconnectErr != nil {
			cancelDisconnect()
			t.Errorf("disconnect MongoDB client accepted without a client certificate: %v", disconnectErr)
		} else {
			cancelDisconnect()
		}
		t.Fatal("external MongoDB endpoint accepted a connection without a required client certificate")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	db, message, err := database.New(ctx, &config, "UTC")
	if err != nil {
		t.Fatalf("connect to external MongoDB with client certificate (TLS setup: %s): %v", message, err)
	}
	defer func() {
		closeCtx, cancelClose := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancelClose()
		if err := db.Mongo.Disconnect(closeCtx); err != nil {
			t.Errorf("disconnect external MongoDB mTLS test client: %v", err)
		}
	}()

	if err := db.Mongo.Ping(ctx, nil); err != nil {
		t.Fatalf("ping external MongoDB over mTLS: %v", err)
	}
	var result struct {
		Version string `bson:"version"`
	}
	if err := db.Mongo.Database("admin").RunCommand(ctx, bson.D{{Key: "buildInfo", Value: 1}}).Decode(&result); err != nil {
		t.Fatalf("query external MongoDB version over mTLS: %v", err)
	}
	t.Logf("External MongoDB mTLS verified: server version %s; connection without client certificate rejected", result.Version)
}
