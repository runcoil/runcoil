package runcoil

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewClientReadsTheEnvironment(t *testing.T) {
	t.Setenv(envURL, "http://localhost:7070")
	t.Setenv(envAPIKey, "rc_api_env")

	client, err := NewClient()
	require.NoError(t, err)
	assert.Equal(t, "rc_api_env", client.apiKey)

	client, err = NewClient(APIKey("rc_api_option"))
	require.NoError(t, err)
	assert.Equal(t, "rc_api_option", client.apiKey, "an option replaces the environment")
}

func TestNewClientValidates(t *testing.T) {
	t.Setenv(envURL, "")
	t.Setenv(envAPIKey, "")

	tests := []struct {
		name string
		opts []ClientOption
		want string
	}{
		{name: "no url", opts: []ClientOption{APIKey("k")}, want: "new client: url is required: set it with URL or RUNCOIL_URL"},
		{name: "bad scheme", opts: []ClientOption{URL("grpc://localhost:7070"), APIKey("k")}, want: `new client: url scheme "grpc" must be http or https`},
		{name: "no host", opts: []ClientOption{URL("http://"), APIKey("k")}, want: "new client: url has no host"},
		{name: "no api key", opts: []ClientOption{URL("http://localhost:7070")}, want: "new client: api key is required: set it with APIKey or RUNCOIL_API_KEY"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := NewClient(tt.opts...)
			assert.EqualError(t, err, tt.want)
		})
	}
}

func TestHTTPClientSpeaksHTTP2Only(t *testing.T) {
	plain, ok := httpClient("http://localhost:7070").Transport.(*http.Transport)
	require.True(t, ok)
	assert.True(t, plain.Protocols.UnencryptedHTTP2())
	assert.False(t, plain.Protocols.HTTP1())

	secure, ok := httpClient("https://runcoil.example").Transport.(*http.Transport)
	require.True(t, ok)
	assert.True(t, secure.Protocols.HTTP2())
	assert.False(t, secure.Protocols.HTTP1())
}
