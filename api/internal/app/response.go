package app

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"log"
	"net/http"
	"strconv"
)

// withSignature signs every response body with SIGNING_SECRET and exposes the
// digest as `X-Signature: sha256=<hex>`. With no secret the middleware is a
// pass-through, which is how the sample proves a sealed variable arrived.
func (application *Application) withSignature(nextHandler http.Handler) http.Handler {
	if application.Secret == "" {
		return nextHandler
	}
	return http.HandlerFunc(func(responseWriter http.ResponseWriter, httpRequest *http.Request) {
		buffered := &bufferWriter{ResponseWriter: responseWriter, status: http.StatusOK}
		nextHandler.ServeHTTP(buffered, httpRequest)
		digest := hmac.New(sha256.New, []byte(application.Secret))
		digest.Write(buffered.body.Bytes())
		responseWriter.Header().Set("X-Signature", "sha256="+hex.EncodeToString(digest.Sum(nil)))
		responseWriter.Header().Set("Content-Length", strconv.Itoa(buffered.body.Len()))
		responseWriter.WriteHeader(buffered.status)
		if _, failure := responseWriter.Write(buffered.body.Bytes()); failure != nil {
			application.Logger.Printf("write signed response: %v", failure)
		}
	})
}

type bufferWriter struct {
	http.ResponseWriter
	status int
	body   bytes.Buffer
}

func (writer *bufferWriter) WriteHeader(statusCode int) {
	writer.status = statusCode
}

func (writer *bufferWriter) Write(payload []byte) (int, error) {
	return writer.body.Write(payload)
}

func writeJSON(responseWriter http.ResponseWriter, statusCode int, payload any) {
	responseWriter.Header().Set("Content-Type", "application/json")
	responseWriter.WriteHeader(statusCode)
	encoder := json.NewEncoder(responseWriter)
	encoder.SetEscapeHTML(false)
	if encodeError := encoder.Encode(payload); encodeError != nil {
		log.Printf("encode response: %v", encodeError)
	}
}

func writeError(responseWriter http.ResponseWriter, statusCode int, message string) {
	writeJSON(responseWriter, statusCode, map[string]string{"error": message})
}
