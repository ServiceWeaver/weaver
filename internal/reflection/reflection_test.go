package reflection_test

import (
	"fmt"
	"io"
	"reflect"
	"testing"

	"github.com/ServiceWeaver/weaver/internal/reflection"
)

func TestType(t *testing.T) {
	for _, test := range []struct {
		want string
		t    reflect.Type
	}{
		{"int", reflection.Type[int]()},
		{"io.Reader", reflection.Type[io.Reader]()},
	} {
		t.Run(test.want, func(t *testing.T) {
			if got := fmt.Sprint(test.t); got != test.want {
				t.Errorf("reflection.Type[io.Reader] = %s, expecting %s", got, test.want)
			}
		})
	}
}
