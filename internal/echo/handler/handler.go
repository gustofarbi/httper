package handler

import (
	"bytes"
	"encoding/json"
	"fmt"
	"image"
	"image/draw"
	"image/jpeg"
	"io"
	"net/http"
	"strings"
)

// NewMux wires every echo route to its handler. It is the single source of
// truth shared by the standalone echo server (cmd/echo) and the in-process
// end-to-end tests.
func NewMux() *http.ServeMux {
	mux := http.NewServeMux()

	mux.HandleFunc("/", CatchAll)
	mux.HandleFunc("GET /http2", Http2)
	mux.HandleFunc("GET /image", Image)
	mux.HandleFunc("GET /bearer", Bearer)
	mux.HandleFunc("GET /basic-auth", BasicAuth)
	mux.HandleFunc("POST /json", JsonBody)
	mux.HandleFunc("POST /form-data", FormData)
	mux.HandleFunc("POST /token", Token)
	mux.HandleFunc("POST /urlencoded", URLEncoded)
	mux.HandleFunc("GET /redirect", Redirect)
	mux.HandleFunc("GET /redirected", Redirected)
	mux.HandleFunc("GET /set-cookie", SetCookie)
	mux.HandleFunc("GET /need-cookie", NeedCookie)

	return mux
}

func URLEncoded(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	for key, values := range r.PostForm {
		_, _ = fmt.Fprintf(w, "%s=%s\n", key, values)
	}
}

// Token issues the bearer token the Bearer handler accepts, so chaining
// fixtures can log in and reuse it.
func Token(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	_, _ = fmt.Fprintln(w, `{"token":"42069"}`)
}

func Redirect(w http.ResponseWriter, r *http.Request) {
	http.Redirect(w, r, "/redirected", http.StatusFound)
}

func Redirected(w http.ResponseWriter, _ *http.Request) {
	_, _ = fmt.Fprintln(w, "Redirected OK")
}

func SetCookie(w http.ResponseWriter, _ *http.Request) {
	http.SetCookie(w, &http.Cookie{Name: "session", Value: "42"})
	_, _ = fmt.Fprintln(w, "Cookie set")
}

func NeedCookie(w http.ResponseWriter, r *http.Request) {
	cookie, err := r.Cookie("session")
	if err != nil || cookie.Value != "42" {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	_, _ = fmt.Fprintln(w, "Cookie OK")
}

func Image(w http.ResponseWriter, _ *http.Request) {
	im := image.NewRGBA(image.Rect(0, 0, 100, 100))
	draw.Draw(im, im.Bounds(), image.White, image.Point{}, draw.Src)
	if err := jpeg.Encode(w, im, nil); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func Http2(w http.ResponseWriter, r *http.Request) {
	_, _ = fmt.Fprintf(w, "Protocol: %s\n", r.Proto)
	if !strings.HasPrefix(r.Proto, "HTTP/2") {
		w.WriteHeader(http.StatusForbidden)
		return
	}
}

func Bearer(w http.ResponseWriter, r *http.Request) {
	const bearerPrefix = "Bearer "
	bearer := r.Header.Get("Authorization")

	if !strings.HasPrefix(bearer, bearerPrefix) {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}

	if strings.TrimPrefix(bearer, bearerPrefix) != "42069" {
		w.WriteHeader(http.StatusForbidden)
		return
	}

	_, _ = fmt.Fprintln(w, "Authorized")
}

func BasicAuth(w http.ResponseWriter, r *http.Request) {
	if u, p, ok := r.BasicAuth(); !ok || u != "foo" || p != "bar" {
		w.Header().Set("WWW-Authenticate", `Basic realm="Restricted"`)
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	_, _ = fmt.Fprintln(w, "Authorized")
}

func JsonBody(w http.ResponseWriter, r *http.Request) {
	content, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	if err = json.NewDecoder(bytes.NewReader(content)).Decode(&struct{}{}); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	_, _ = fmt.Fprintf(w, "Content-length: %d\n", len(content))
}

func FormData(w http.ResponseWriter, r *http.Request) {
	reader, err := r.MultipartReader()
	if err == nil && reader != nil {
		for {
			part, err := reader.NextPart()
			if err != nil {
				break
			}

			_, _ = fmt.Fprintf(w, "Part: %s, '%s'\n", part.FormName(), part.FileName())

			content, err := io.ReadAll(part)
			if err != nil {
				_, _ = fmt.Fprintf(w, "Error reading part: %s\n", err)
				continue
			}

			if part.FileName() == "" {
				_, _ = fmt.Fprintln(w, string(content))
			}

			if r.URL.Query().Has("debug") {
				_, _ = fmt.Fprintln(w)
			}

			if r.URL.Query().Has("headers") {
				for k, v := range part.Header {
					_, _ = fmt.Fprintf(w, "%s: %s\n", k, v)
				}
			}

			_, _ = fmt.Fprintf(w, "Content-length: %d\n", len(content))
		}
	}
}

func CatchAll(w http.ResponseWriter, r *http.Request) {
	if r.URL.Query().Has("query") {
		for k, v := range r.URL.Query() {
			_, _ = fmt.Fprintf(w, "%s: %s\n", k, v)
		}
	}
}
