/*
Copyright 2019 The Kubernetes Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package value

import (
	"reflect"
	"testing"
)

func TestReflectPrimitives(t *testing.T) {

	rv := ReflectValue{"string"}
	if !rv.IsString() {
		t.Error("expected IsString to be true")
	}
	if rv.String() != "string" {
		t.Errorf("expected rv.String to be 'string' but got %s", rv.String())
	}

	rv = ReflectValue{1}
	if !rv.IsInt() {
		t.Error("expected IsInt to be true")
	}
	if rv.Int() != 1 {
		t.Errorf("expected rv.Int to be 1 but got %s", rv.String())
	}
}

func TestReflectPointers(t *testing.T) {
	s := "string"
	rv := ReflectValue{&s}
	if !rv.IsString() {
		t.Error("expected IsString to be true")
	}
	if rv.String() != "string" {
		t.Errorf("expected rv.String to be 'string' but got %s", rv.String())
	}
}

// TODO: test Set, Delete
func TestReflectStruct(t *testing.T) {
	cases := []struct{
		name string
		val interface{}
		expectedMap map[string]interface{}
	} {
		{
			name: "struct",
			val: struct{I int64; S string} {I: 10, S: "string"},
			expectedMap: map[string]interface{}{"I": int64(10), "S": "string"},
		},
		{
			name: "structPtr",
			val: &struct{I int64; S string} {I: 10, S: "string"},
			expectedMap: map[string]interface{}{"I": int64(10), "S": "string"},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rv := ReflectValue{tc.val}
			if !rv.IsMap() {
				t.Error("expected IsMap to be true")
			}
			m := rv.Map()
			if i, ok := m.Get("I"); ok {
				if !i.IsInt() {
					t.Errorf("expected I to be an int, but got: %T", i.Interface())
				} else if i.Int() != 10 {
					t.Errorf("expected I to be 10 but got: %v", i)
				}
			} else {
				t.Error("expected I to be in map")
			}
			if m.Length() != len(tc.expectedMap) {
				t.Errorf("expected map to be of length %d but got %d", len(tc.expectedMap), m.Length())
			}
			iterateResult := map[string]interface{}{}
			m.Iterate(func(s string, value Value) bool {
				iterateResult[s] = value.Interface()
				return true
			})
			if !reflect.DeepEqual(iterateResult, tc.expectedMap) {
				t.Errorf("expected iterate to produce %#v but got %#v", tc.expectedMap, iterateResult)
			}
		})
	}
}

// TODO: test Set, Delete
func TestReflectMap(t *testing.T) {
	cases := []struct{
		name string
		val interface{}
		length int
	} {
		{
			name: "map",
			val: map[string]string{"key1": "value1"},
			length: 1,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rv := ReflectValue{tc.val}
			if !rv.IsMap() {
				t.Error("expected IsMap to be true")
			}
			m := rv.Map()
			if i, ok := m.Get("key1"); ok {
				if !i.IsString() {
					t.Errorf("expected key1 to be an string, but got: %T", i.Interface())
				} else if i.String() != "value1" {
					t.Errorf("expected value of key1 to be value1 but got: %v", i)
				}
			} else {
				t.Error("expected key1 to be in map")
			}
			if m.Length() != tc.length {
				t.Errorf("expected map to be of length %d but got %d", tc.length, m.Length())
			}
			iterateResult := map[string]string{}
			m.Iterate(func(s string, value Value) bool {
				iterateResult[s] = value.String()
				return true
			})
			if !reflect.DeepEqual(iterateResult, tc.val) {
				t.Errorf("expected iterate to produce %#v but got %#v", tc.val, iterateResult)
			}
		})
	}
}

func TestReflectList(t *testing.T) {
	cases := []struct{
		name string
		val interface{}
		length int
	} {
		{
			name: "list",
			val: []string{"value1", "value2"},
			length: 2,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rv := ReflectValue{tc.val}
			if !rv.IsList() {
				t.Error("expected IsList to be true")
			}
			m := rv.List()
			i := m.At(0)
			if !i.IsString() {
				t.Errorf("expected index 0 to be an string, but got: %v", reflect.TypeOf(i.Interface()))
			} else if i.String() != "value1" {
				t.Errorf("expected index 0 to be value1 but got: %v", i)
			}
			if m.Length() != tc.length {
				t.Errorf("expected list to be of length %d but got %d", tc.length, m.Length())
			}
			iterateResult := make([]string, m.Length())
			m.Iterate(func(i int, v Value) {
				iterateResult[i] = v.String()
			})
			if !reflect.DeepEqual(iterateResult, tc.val) {
				t.Errorf("expected iterate to produce %#v but got %#v", tc.val, iterateResult)
			}
		})
	}
}