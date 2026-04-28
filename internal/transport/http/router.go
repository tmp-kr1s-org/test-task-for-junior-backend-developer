package transporthttp

import (
	"net/http"

	"github.com/gorilla/mux"

	swaggerdocs "example.com/taskservice/internal/transport/http/docs"
	httphandlers "example.com/taskservice/internal/transport/http/handlers"
)

func NewRouter(taskHandler *httphandlers.TaskHandler, docsHandler *swaggerdocs.Handler) *mux.Router {
	router := mux.NewRouter().StrictSlash(true)

	router.HandleFunc("/swagger/openapi.json", docsHandler.ServeSpec).Methods(http.MethodGet)
	router.HandleFunc("/swagger/", docsHandler.ServeUI).Methods(http.MethodGet)
	router.HandleFunc("/swagger", docsHandler.RedirectToUI).Methods(http.MethodGet)

	api := router.PathPrefix("/api/v1").Subrouter()

	api.HandleFunc("/tasks", taskHandler.Create).Methods(http.MethodPost)
	api.HandleFunc("/tasks", taskHandler.List).Methods(http.MethodGet)
	api.HandleFunc("/tasks/{id:[0-9]+}", taskHandler.GetByID).Methods(http.MethodGet)
	api.HandleFunc("/tasks/{id:[0-9]+}", taskHandler.Update).Methods(http.MethodPut)
	api.HandleFunc("/tasks/{id:[0-9]+}", taskHandler.Delete).Methods(http.MethodDelete)

	api.HandleFunc("/tasks/{id:[0-9]+}/occurrences/{date}", taskHandler.UpsertOverride).Methods(http.MethodPut)
	api.HandleFunc("/tasks/{id:[0-9]+}/occurrences/{date}", taskHandler.CancelOccurrence).Methods(http.MethodDelete)
	api.HandleFunc("/tasks/{id:[0-9]+}/fork", taskHandler.Fork).Methods(http.MethodPost)

	// Adding sibling routes (e.g. /tasks/{id}/fork) makes gorilla/mux's automatic
	// 405-vs-404 detection misfire, returning 404 for unsupported methods on a
	// known path. We walk the routes ourselves to restore the legacy contract.
	notFound := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if pathHasAnotherMethod(router, r) {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		http.NotFound(w, r)
	})
	router.NotFoundHandler = notFound
	api.NotFoundHandler = notFound

	return router
}

// pathHasAnotherMethod returns true if the request path matches any registered
// route whose declared method differs from the request's method. We walk only
// terminal routes (those with declared methods) — PathPrefix-only parent routes
// are skipped to avoid false positives.
func pathHasAnotherMethod(router *mux.Router, r *http.Request) bool {
	found := false
	_ = router.Walk(func(route *mux.Route, _ *mux.Router, _ []*mux.Route) error {
		methods, err := route.GetMethods()
		if err != nil || len(methods) == 0 {
			return nil
		}
		differs := false
		for _, m := range methods {
			if m != r.Method {
				differs = true
				break
			}
		}
		if !differs {
			return nil
		}
		probe := &http.Request{Method: methods[0], URL: r.URL, Host: r.Host, Header: r.Header}
		var match mux.RouteMatch
		if route.Match(probe, &match) {
			found = true
		}
		return nil
	})
	return found
}
