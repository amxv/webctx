package handler

import (
	"net/http"

	"github.com/amxv/webctx/pkg/origo"
)

// Handler is deployed as a serverless Go Function in the Origo Vercel project.
// It imports a public package because Vercel compiles the function in a
// synthetic package, which cannot directly import Go internal packages.
func Handler(w http.ResponseWriter, r *http.Request) {
	origo.Handler().ServeHTTP(w, r)
}
