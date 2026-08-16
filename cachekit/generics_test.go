package cachekit_test

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/mazzama/go-bootkit/cachekit"
	"github.com/mazzama/go-bootkit/cachekit/memcache"
)

type User struct {
	ID   string
	Name string
}

func TestGenerics_ValueType(t *testing.T) {
	c := memcache.New()
	ctx := context.Background()

	u := User{ID: "123", Name: "Alice"}
	err := cachekit.Set(ctx, c, "user:123", u, time.Minute)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	ret, err := cachekit.Get[User](ctx, c, "user:123")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if ret.Name != "Alice" {
		t.Errorf("expected Alice, got %s", ret.Name)
	}
}

// strictCodec simulates a codec that does not automatically allocate double pointers
// (unlike encoding/json) to prove that the generic helper does not protect against
// pointer types.
type strictCodec struct{}

func (strictCodec) Marshal(v any) ([]byte, error) {
	return json.Marshal(v)
}

func (strictCodec) Unmarshal(data []byte, v any) error {
	// v is always a pointer because we pass &zero.
	// If the inner type (zero) is ALSO a pointer, we reject it.
	if reflect.TypeOf(v).Elem().Kind() == reflect.Ptr {
		return errors.New("strictCodec: pointer to pointer not allowed")
	}
	return json.Unmarshal(data, v)
}

func TestGenerics_PointerType_Fails(t *testing.T) {
	// Although standard encoding/json natively allocates **T, the ADR states
	// passing a pointer type should fail, relying on the codec to reject it.
	// We use a strict codec to prove the generic helper itself does not
	// safeguard against pointer types.
	c := memcache.New(memcache.WithCodec(strictCodec{}))
	ctx := context.Background()

	u := User{ID: "123", Name: "Alice"}
	err := cachekit.Set(ctx, c, "user:123", u, time.Minute)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	_, err = cachekit.Get[*User](ctx, c, "user:123")
	if err == nil {
		t.Fatalf("expected error when passing pointer type, got nil")
	}

	if err.Error() != "strictCodec: pointer to pointer not allowed" {
		t.Errorf("unexpected error message: %v", err)
	}
}
