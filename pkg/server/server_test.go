package server

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestServerServesLiveEndpoint(t *testing.T) {
	s, err := NewHttpServer(0) // :0 => random free port
	require.NoError(t, err)

	ctx, cancel := context.WithCancel(context.Background())
	go func() { _ = s.Run(ctx) }()
	defer cancel()

	time.Sleep(100 * time.Millisecond)
	resp, err := http.Get("http://" + s.Addr().String() + "/live")
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()
	assert.Equal(t, http.StatusOK, resp.StatusCode)
}
