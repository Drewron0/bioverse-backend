package httperr

import (
	"encoding/json"
	"net/http"
	"strconv"
)

// ProblemDetails represents an RFC 7807 problem details response.
type ProblemDetails struct {
	Type     string            `json:"type"`
	Title    string            `json:"title"`
	Status   int               `json:"status"`
	Detail   string            `json:"detail"`
	Instance string            `json:"instance,omitempty"`
	Code     string            `json:"code"`
	Errors   map[string]string `json:"errors,omitempty"`
}

// Respond writes a ProblemDetails struct as JSON with application/problem+json Content-Type.
func Respond(w http.ResponseWriter, r *http.Request, status int, code, title, detail string, validationErrors map[string]string) {
	w.Header().Set("Content-Type", "application/problem+json")
	w.WriteHeader(status)

	problem := ProblemDetails{
		Type:     "about:blank",
		Title:    title,
		Status:   status,
		Detail:   detail,
		Instance: r.URL.Path,
		Code:     code,
		Errors:   validationErrors,
	}

	_ = json.NewEncoder(w).Encode(problem)
}

// BadRequest writes a 400 Bad Request problem response.
func BadRequest(w http.ResponseWriter, r *http.Request, detail string, errors map[string]string) {
	Respond(w, r, http.StatusBadRequest, "BAD_REQUEST", "Bad Request", detail, errors)
}

// Unauthorized writes a 401 Unauthorized problem response.
func Unauthorized(w http.ResponseWriter, r *http.Request, detail string) {
	Respond(w, r, http.StatusUnauthorized, "UNAUTHORIZED", "Unauthorized", detail, nil)
}

// UnauthorizedWithCode writes a 401 Unauthorized problem response with custom code.
func UnauthorizedWithCode(w http.ResponseWriter, r *http.Request, code, detail string) {
	Respond(w, r, http.StatusUnauthorized, code, "Unauthorized", detail, nil)
}

// Forbidden writes a 403 Forbidden problem response.
func Forbidden(w http.ResponseWriter, r *http.Request, detail string) {
	Respond(w, r, http.StatusForbidden, "FORBIDDEN", "Forbidden", detail, nil)
}

// NotFound writes a 404 Not Found problem response.
func NotFound(w http.ResponseWriter, r *http.Request, detail string) {
	Respond(w, r, http.StatusNotFound, "NOT_FOUND", "Resource Not Found", detail, nil)
}

// Conflict writes a 409 Conflict problem response.
func Conflict(w http.ResponseWriter, r *http.Request, detail string) {
	Respond(w, r, http.StatusConflict, "CONFLICT", "Conflict", detail, nil)
}

// ConflictWithCode writes a 409 Conflict problem response with custom stable code.
func ConflictWithCode(w http.ResponseWriter, r *http.Request, code, detail string) {
	Respond(w, r, http.StatusConflict, code, "Conflict", detail, nil)
}

// UnprocessableEntity writes a 422 Unprocessable Entity problem response.
func UnprocessableEntity(w http.ResponseWriter, r *http.Request, detail string, errors map[string]string) {
	Respond(w, r, http.StatusUnprocessableEntity, "UNPROCESSABLE_ENTITY", "Unprocessable Entity", detail, errors)
}

// TooManyRequests writes a 429 Too Many Requests problem response with Retry-After header.
func TooManyRequests(w http.ResponseWriter, r *http.Request, retryAfterSeconds int, detail string) {
	if retryAfterSeconds > 0 {
		w.Header().Set("Retry-After", strconv.Itoa(retryAfterSeconds))
	}
	Respond(w, r, http.StatusTooManyRequests, "RATE_LIMIT_EXCEEDED", "Too Many Requests", detail, nil)
}

// ServiceUnavailable writes a 503 Service Unavailable problem response.
func ServiceUnavailable(w http.ResponseWriter, r *http.Request, detail string) {
	Respond(w, r, http.StatusServiceUnavailable, "SERVICE_UNAVAILABLE", "Service Unavailable", detail, nil)
}

// InternalServerError writes a 500 Internal Server Error problem response.
func InternalServerError(w http.ResponseWriter, r *http.Request) {
	Respond(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", "Internal Server Error", "An unexpected error occurred.", nil)
}
