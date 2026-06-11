package request

import (
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/textproto"
	"net/url"
	"slices"
	"strings"
)

var (
	methods = []string{
		http.MethodGet,
		http.MethodHead,
		http.MethodOptions,
		http.MethodDelete,
		http.MethodPatch,
		http.MethodPost,
		http.MethodPut,
		http.MethodConnect,
		"GRPC",
		"WEBSOCKET",
		"GRAPHQL",
	}

	protos = []string{
		"HTTP/1.1",
		"HTTP/2",
		"HTTP/2 (Prior Knowledge)",
	}
)

func buildRequest(essentialsRaw, headersRaw, bodyRaw, wd string) (*http.Request, error) {
	headers := parseHeaders(headersRaw)
	body, err := parseBody(
		headers.Get("Content-Type"),
		bodyRaw,
		wd,
	)
	if err != nil {
		return nil, fmt.Errorf("getting request body: %w", err)
	}

	lines := strings.Split(essentialsRaw, "\n")
	if len(lines) < 1 {
		return nil, fmt.Errorf("too few lines in the header of the file: %s", essentialsRaw)
	}

	method, parsedUrl, proto := parseEssentials(lines[0])
	if parsedUrl == nil {
		return nil, fmt.Errorf("could not parse url from: %s", lines[0])
	}

	request, err := http.NewRequest(method, parsedUrl.String(), body)
	if err != nil {
		return nil, fmt.Errorf("cannot create a request: %w", err)
	}

	if proto != "" {
		request.Proto = proto
	}

	transferHeaders(request, headers)

	return request, nil
}

func parseEssentials(essentialsRaw string) (
	method string,
	parsedUrl *url.URL,
	proto string,
) {
	for _, part := range strings.Split(essentialsRaw, " ") {
		part = strings.TrimSpace(part)

		if method == "" {
			if parsed := parseMethod(part); parsed != "" {
				method = parsed
				continue
			}
		}

		if parsedUrl == nil {
			if parsed := parseUrl(part); parsed != nil {
				parsedUrl = parsed
				continue
			}
		}

		if proto == "" {
			if parsed := parseProto(part); parsed != "" {
				proto = part
				continue
			}
		}
	}

	if method == "" {
		method = http.MethodGet
	}

	return
}

func parseProto(raw string) string {
	if slices.Contains(protos, raw) {
		return raw
	}
	return ""
}

func parseUrl(raw string) *url.URL {
	if parsed, err := url.Parse(raw); err == nil {
		return parsed
	}

	return nil
}

func parseMethod(raw string) string {
	if slices.Contains(methods, raw) {
		return raw
	}

	return ""
}

func transferHeaders(request *http.Request, headers textproto.MIMEHeader) {
	for k := range headers {
		value := headers.Get(k)
		if strings.ToLower(k) != "authorization" {
			request.Header.Add(k, value)
		}

		parts := strings.Split(value, " ")

		if len(parts) < 2 {
			continue
		}

		if strings.ToLower(parts[0]) != "basic" {
			request.Header.Add(k, value)
			continue
		}

		var password string
		if len(parts) > 2 {
			password = parts[2]
		}

		request.SetBasicAuth(parts[1], password)
	}
}

func parseBody(contentType, body, wd string) (io.Reader, error) {
	if contentType == "" {
		return nil, nil
	}

	contentType, boundary := splitContentType(contentType)

	switch contentType {
	case "application/json":
		return getJSONBody(body), nil
	case "multipart/form-data":
		return getFormDataBody(boundary, body, wd)
	default:
		slog.Warn("unknown content-type", "content-type", contentType)
		return nil, nil
	}
}

func getJSONBody(body string) io.Reader {
	return strings.NewReader(body)
}

func parseHeaders(headersRaw string) textproto.MIMEHeader {
	result := make(textproto.MIMEHeader)

	if strings.TrimSpace(headersRaw) == "" {
		return result
	}

	for _, header := range strings.Split(headersRaw, "\n") {
		key, value, ok := strings.Cut(header, ":")
		if !ok {
			slog.Warn("cannot parse header", "header", header)
			continue
		}

		result.Add(strings.TrimSpace(key), strings.TrimSpace(value))
	}

	return result
}
