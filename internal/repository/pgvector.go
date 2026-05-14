package repository

import (
	"fmt"
	"strings"
)

type Vector []float32

func (v Vector) String() string {
	if len(v) == 0 {
		return "NULL"
	}
	parts := make([]string, len(v))
	for i, f := range v {
		parts[i] = fmt.Sprintf("%g", f)
	}
	return "[" + strings.Join(parts, ",") + "]"
}

func (v Vector) Value() (interface{}, error) {
	if len(v) == 0 {
		return nil, nil
	}
	return v.String(), nil
}
