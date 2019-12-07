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
	"encoding/json"
	"reflect"
	"testing"
	"time"
)

func TestReflectPrimitives(t *testing.T) {

	rv := MustReflect("string")
	if !rv.IsString() {
		t.Error("expected IsString to be true")
	}
	if rv.String() != "string" {
		t.Errorf("expected rv.String to be 'string' but got %s", rv.String())
	}

	rv = MustReflect(1)
	if !rv.IsInt() {
		t.Error("expected IsInt to be true")
	}
	if rv.Int() != 1 {
		t.Errorf("expected rv.Int to be 1 but got %s", rv.String())
	}
}

type Convertable struct {
	Value string
}

func (t Convertable) MarshalJSON() ([]byte, error) {
	return json.Marshal(t.Value)
}

func (t Convertable) UnmarshalJSON(data []byte) error {
	return json.Unmarshal(data, &t.Value)
}

type PtrConvertable struct {
	Value string
}

func (t *PtrConvertable) MarshalJSON() ([]byte, error) {
	return json.Marshal(t.Value)
}

func (t *PtrConvertable) UnmarshalJSON(data []byte) error {
	return json.Unmarshal(data, &t.Value)
}

func TestReflectCustomStringConversion(t *testing.T) {
	dateTime, err := time.Parse(time.RFC3339, "2006-01-02T15:04:05+07:00")
	if err != nil {
		t.Fatal(err)
	}
	cases := []struct{
		name string
		convertable interface{}
		expected string
	} {
		{
			name: "marshalable-struct",
			convertable: Convertable{Value: "struct-test"},
			expected: "struct-test",
		},
		{
			name: "marshalable-pointer",
			convertable: &PtrConvertable{Value: "pointer-test"},
			expected: "pointer-test",
		},
		{
			name: "pointer-to-marshalable-struct",
			convertable: &Convertable{Value: "pointer-test"},
			expected: "pointer-test",
		},
		{
			name: "time",
			convertable: dateTime,
			expected: "2006-01-02T15:04:05+07:00",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rv := MustReflect(tc.convertable)
			if !rv.IsString() {
				t.Fatalf("expected IsString to be true, but kind is: %T", rv.Interface())
			}
			if rv.String() != tc.expected {
				t.Errorf("expected rv.String to be %v but got %s", tc.expected, rv.String())
			}
		})
	}
}

func TestReflectPointers(t *testing.T) {
	s := "string"
	rv := MustReflect(&s)
	if !rv.IsString() {
		t.Error("expected IsString to be true")
	}
	if rv.String() != "string" {
		t.Errorf("expected rv.String to be 'string' but got %s", rv.String())
	}
}

type T struct {
	I int64 `json:"int"`
}

type testReflectStruct struct{I int64 `json:"int"`; S string}
type testInlineStruct struct{Inline T `json:",inline"`; S string}
type testOmitemptyStruct struct{Noomit *string `json:"noomit"`; Omit *string `json:"omit,omitempty"`}

// TODO: test Set, Delete
func TestReflectStruct(t *testing.T) {
	cases := []struct{
		name string
		val interface{}
		expectedMap map[string]interface{}
	} {
		{
			name: "struct",
			val: testReflectStruct{I: 10, S: "string"},
			expectedMap: map[string]interface{}{"int": int64(10), "S": "string"},
		},
		{
			name: "structPtr",
			val: &testReflectStruct{I: 10, S: "string"},
			expectedMap: map[string]interface{}{"int": int64(10), "S": "string"},
		},
		{
			name: "inline",
			val: &testInlineStruct {Inline: T{I: 10}, S: "string"},
			expectedMap: map[string]interface{}{"int": int64(10), "S": "string"},
		},
		{
			name: "omitempty",
			val: testOmitemptyStruct {Noomit: nil, Omit: nil},
			expectedMap: map[string]interface{}{"noomit": (*string)(nil)},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rv := MustReflect(tc.val)
			if !rv.IsMap() {
				t.Error("expected IsMap to be true")
			}
			m := rv.Map()
			if i, ok := m.Get("int"); ok {
				if !i.IsInt() {
					t.Errorf("expected I to be an int, but got: %T", i.Interface())
				} else if i.Int() != 10 {
					t.Errorf("expected I to be 10 but got: %v", i)
				}
			}
			if m.Length() != len(tc.expectedMap) {
				t.Errorf("expected map to be of length %d but got %d", len(tc.expectedMap), m.Length())
			}
			iterateResult := map[string]interface{}{}
			m.Iterate(func(s string, value Value) bool {
				iterateResult[s] = value.(*reflectValue).Value.Interface()
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
			rv := MustReflect(tc.val)
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
			rv := MustReflect(tc.val)
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
		})
	}
}