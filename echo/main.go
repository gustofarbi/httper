package main

import (
	"echo/internal/handler"
	"fmt"
	"net/http"
	"os"
	"path"
)

const address = ":8080"

func main() {
	mux := http.NewServeMux()

	mux.HandleFunc("/", handler.CatchAll)
	mux.HandleFunc("GET /http2", handler.Http2)
	mux.HandleFunc("GET /image", handler.Image)
	mux.HandleFunc("GET /bearer", handler.Bearer)
	mux.HandleFunc("GET /basic-auth", handler.BasicAuth)
	mux.HandleFunc("POST /json", handler.JsonBody)
	mux.HandleFunc("POST /form-data", handler.FormData)

	wd, err := os.Getwd()
	if err != nil {
		panic(err)
	}

	fmt.Println("Listening on", address)

	certFile := path.Join(wd, "certs/localhost+2.pem")
	keyFile := path.Join(wd, "certs/localhost+2-key.pem")

	if err = http.ListenAndServeTLS(
		address,
		certFile,
		keyFile,
		mux,
	); err != nil {
		panic(err)
	}
}
