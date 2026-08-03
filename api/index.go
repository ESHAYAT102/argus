package handler

import (
	"net/http"

	"github.com/argus-env/argus/vercelapp"
)

// Handler is the Vercel Go Function entrypoint. The implementation lives in a
// public module package because Vercel compiles this file as `handler/api`, a
// synthetic import path that Go does not permit to import Argus internal packages.
func Handler(writer http.ResponseWriter, request *http.Request) {
	vercelapp.Handler(writer, request)
}
