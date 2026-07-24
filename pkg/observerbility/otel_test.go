package observerbility

import (
	"context"
	"testing"
)

func TestNewTraceProvider_SchemaURLCompatibleWithDefault(t *testing.T) {
	// resource.Merge fails with "conflicting Schema URL" when semconv.SchemaURL
	// doesn't match the schema URL built into resource.Default(). This test
	// catches version mismatches early (e.g. semconv/v1.37.0 vs v1.40.0).
	ctx := context.Background()
	provider, err := newTraceProvider(ctx)
	if err != nil {
		t.Fatalf("newTraceProvider failed – likely a semconv schema URL mismatch with resource.Default(): %v", err)
	}
	_ = provider.Shutdown(ctx)
}
