package http

import (
	"net/http"
	"strconv"

	"github.com/deploykitdev/deploykit"
)

// parseQueryInt returns the int value of query param key, or def if the param
// is unset. Returns an EINVALID error if the param is present but not a valid
// integer.
func parseQueryInt(r *http.Request, key string, def int) (int, error) {
	v := r.URL.Query().Get(key)
	if v == "" {
		return def, nil
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return 0, deploykit.Errorf(deploykit.EINVALID, "Query param %q must be an integer.", key)
	}
	return n, nil
}

// parseQueryString returns a pointer to the value of query param key, or nil
// if the param is unset. Useful for optional filter params.
func parseQueryString(r *http.Request, key string) *string {
	v := r.URL.Query().Get(key)
	if v == "" {
		return nil
	}
	return &v
}
