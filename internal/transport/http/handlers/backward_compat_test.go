package handlers_test

// Backward-compatibility tests.
//
// These tests pin the exact JSON shape of the public API as it existed
// before the recurring-tasks feature. Their job is to fail loudly the moment
// somebody renames, removes, or retypes a field that an old client (mobile
// app, third-party integration) might still depend on.
//
// Adding *new* optional fields to responses is fine — these tests only check
// that the *old* fields are still there with the *old* names and types.

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
	"time"

	taskdomain "example.com/taskservice/internal/domain/task"
	taskusecase "example.com/taskservice/internal/usecase/task"
)

// expected top-level fields of a single Task object in any response.
var taskFieldTypes = map[string]string{
	"id":          "number",
	"title":       "string",
	"description": "string",
	"status":      "string",
	"created_at":  "string",
	"updated_at":  "string",
}

// jsonType reports the JSON type-name of v as produced by encoding/json
// when decoded into interface{}.
func jsonType(v any) string {
	switch v.(type) {
	case nil:
		return "null"
	case bool:
		return "bool"
	case float64:
		return "number"
	case string:
		return "string"
	case []any:
		return "array"
	case map[string]any:
		return "object"
	default:
		return "unknown"
	}
}

// assertTaskShape verifies that obj has exactly the legacy task fields, with
// the legacy types, and no missing fields. Extra fields are allowed (forward-
// compatible), missing or retyped fields are not.
func assertTaskShape(t *testing.T, obj map[string]any) {
	t.Helper()
	for key, wantType := range taskFieldTypes {
		v, ok := obj[key]
		if !ok {
			t.Errorf("backward-compat: field %q missing from response (old clients depend on it)", key)
			continue
		}
		if got := jsonType(v); got != wantType {
			t.Errorf("backward-compat: field %q has type %q, expected %q (old clients will break)", key, got, wantType)
		}
	}

	// timestamps must remain RFC3339 — that is what old clients parse.
	for _, key := range []string{"created_at", "updated_at"} {
		s, ok := obj[key].(string)
		if !ok {
			continue
		}
		if _, err := time.Parse(time.RFC3339, s); err != nil {
			if _, err2 := time.Parse(time.RFC3339Nano, s); err2 != nil {
				t.Errorf("backward-compat: field %q = %q is not RFC3339-parseable: %v", key, s, err)
			}
		}
	}
}

func sampleDomainTask() *taskdomain.Task {
	return &taskdomain.Task{
		ID:          1,
		Title:       "buy milk",
		Description: "2L",
		Status:      taskdomain.StatusNew,
		CreatedAt:   time.Date(2026, 4, 25, 10, 0, 0, 0, time.UTC),
		UpdatedAt:   time.Date(2026, 4, 25, 10, 0, 0, 0, time.UTC),
	}
}

func TestBackwardCompat_Create_ResponseShape(t *testing.T) {
	uc := &stubUsecase{
		createFn: func(_ context.Context, in taskusecase.CreateInput) (*taskdomain.Task, error) {
			tsk := sampleDomainTask()
			tsk.Title = in.Title
			tsk.Description = in.Description
			tsk.Status = in.Status
			return tsk, nil
		},
	}
	srv := newServer(uc)

	rec := doJSON(t, srv, http.MethodPost, "/api/v1/tasks", map[string]any{
		"title":       "buy milk",
		"description": "2L",
		"status":      "new",
	})
	if rec.Code != http.StatusCreated {
		t.Fatalf("status: want 201, got %d", rec.Code)
	}
	var body map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("response is not a JSON object: %v (body=%s)", err, rec.Body)
	}
	assertTaskShape(t, body)
}

func TestBackwardCompat_GetByID_ResponseShape(t *testing.T) {
	uc := &stubUsecase{
		getFn: func(context.Context, int64) (*taskdomain.Task, error) {
			return sampleDomainTask(), nil
		},
	}
	srv := newServer(uc)

	rec := doJSON(t, srv, http.MethodGet, "/api/v1/tasks/1", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("status: want 200, got %d", rec.Code)
	}
	var body map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("response is not a JSON object: %v", err)
	}
	assertTaskShape(t, body)
}

func TestBackwardCompat_Update_ResponseShape(t *testing.T) {
	uc := &stubUsecase{
		updateFn: func(_ context.Context, id int64, in taskusecase.UpdateInput) (*taskdomain.Task, error) {
			return &taskdomain.Task{
				ID: id, Title: in.Title, Description: in.Description, Status: in.Status,
				CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC(),
			}, nil
		},
	}
	srv := newServer(uc)

	rec := doJSON(t, srv, http.MethodPut, "/api/v1/tasks/1", map[string]any{
		"title": "new", "description": "d", "status": "done",
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("status: want 200, got %d", rec.Code)
	}
	var body map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("response is not a JSON object: %v", err)
	}
	assertTaskShape(t, body)
}

func TestBackwardCompat_List_ResponseShape(t *testing.T) {
	uc := &stubUsecase{
		listFn: func(context.Context) ([]taskdomain.Task, error) {
			return []taskdomain.Task{*sampleDomainTask(), *sampleDomainTask()}, nil
		},
	}
	srv := newServer(uc)

	rec := doJSON(t, srv, http.MethodGet, "/api/v1/tasks", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("status: want 200, got %d", rec.Code)
	}
	var arr []map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &arr); err != nil {
		t.Fatalf("response is not a JSON array: %v", err)
	}
	if len(arr) != 2 {
		t.Fatalf("want 2 items, got %d", len(arr))
	}
	for i, item := range arr {
		t.Run("item-"+itoa(i), func(t *testing.T) {
			assertTaskShape(t, item)
		})
	}
}

// TestBackwardCompat_Delete_NoBody pins the contract that DELETE returns 204
// with an empty body. Old clients may rely on the absence of a body.
func TestBackwardCompat_Delete_NoBody(t *testing.T) {
	uc := &stubUsecase{
		deleteFn: func(context.Context, int64) error { return nil },
	}
	srv := newServer(uc)
	rec := doJSON(t, srv, http.MethodDelete, "/api/v1/tasks/1", nil)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("status: want 204, got %d", rec.Code)
	}
	if rec.Body.Len() != 0 {
		t.Errorf("backward-compat: 204 response must have empty body, got %q", rec.Body.String())
	}
}

// TestBackwardCompat_ErrorEnvelope pins the {"error": "..."} envelope shape.
// If we ever switch error reporting (e.g. to {"code":"...","message":"..."}),
// this test makes the change visible.
func TestBackwardCompat_ErrorEnvelope(t *testing.T) {
	uc := &stubUsecase{
		getFn: func(context.Context, int64) (*taskdomain.Task, error) {
			return nil, taskdomain.ErrNotFound
		},
	}
	srv := newServer(uc)
	rec := doJSON(t, srv, http.MethodGet, "/api/v1/tasks/999", nil)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status: want 404, got %d", rec.Code)
	}
	var body map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("error response is not JSON: %v", err)
	}
	v, ok := body["error"]
	if !ok {
		t.Fatalf("backward-compat: error response must contain \"error\" field, got %v", body)
	}
	if jsonType(v) != "string" {
		t.Errorf("backward-compat: error field must be a string, got %s", jsonType(v))
	}
}

// TestBackwardCompat_OmittingOptionalFieldsOnCreate ensures that an old client
// which only sends `title` (omitting description and status) still gets a 201.
// This guards against a future change that accidentally makes a new optional
// field required.
func TestBackwardCompat_OmittingOptionalFieldsOnCreate(t *testing.T) {
	uc := &stubUsecase{
		createFn: func(_ context.Context, in taskusecase.CreateInput) (*taskdomain.Task, error) {
			if in.Title != "x" {
				t.Errorf("title not propagated: %q", in.Title)
			}
			return &taskdomain.Task{ID: 1, Title: in.Title, Status: taskdomain.StatusNew}, nil
		},
	}
	srv := newServer(uc)
	// minimal legacy body: only title.
	rec := doJSON(t, srv, http.MethodPost, "/api/v1/tasks", `{"title":"x"}`)
	if rec.Code != http.StatusCreated {
		t.Fatalf("backward-compat: legacy minimal body must yield 201, got %d (body=%s)", rec.Code, rec.Body)
	}
}

// itoa is a tiny helper to avoid importing strconv just for subtest names.
func itoa(i int) string {
	if i == 0 {
		return "0"
	}
	var buf [20]byte
	pos := len(buf)
	neg := i < 0
	if neg {
		i = -i
	}
	for i > 0 {
		pos--
		buf[pos] = byte('0' + i%10)
		i /= 10
	}
	if neg {
		pos--
		buf[pos] = '-'
	}
	return string(buf[pos:])
}
