package request

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestIsGRPC(t *testing.T) {
	grpc := &Template{Essentials: "GRPC localhost:8081/grpcecho.EchoService/Echo"}
	assert.True(t, grpc.IsGRPC())

	http := &Template{Essentials: "GET https://example.com/"}
	assert.False(t, http.IsGRPC())
}

func TestBuildGRPC(t *testing.T) {
	template := &Template{
		Essentials: "GRPC {{host}}/grpcecho.EchoService/Echo",
		HeadersRaw: "X-Token: {{token}}",
		BodyRaw:    `{"message": "{{msg}}"}`,
	}

	vars := map[string]string{"host": "localhost:8081", "token": "secret", "msg": "hi"}
	resolve := func(s string) string {
		for key, value := range vars {
			s = strings.ReplaceAll(s, "{{"+key+"}}", value)
		}
		return s
	}

	rawURL, headers, body := template.BuildGRPC(resolve)

	assert.Equal(t, "localhost:8081/grpcecho.EchoService/Echo", rawURL)
	require.Len(t, headers, 1)
	assert.Equal(t, [2]string{"X-Token", "secret"}, headers[0])
	assert.Equal(t, `{"message": "hi"}`, body)
}
