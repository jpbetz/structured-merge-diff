/*
Copyright 2018 The Kubernetes Authors.

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

package typed_test

import (
	"fmt"
	"strings"
	"testing"

	"sigs.k8s.io/structured-merge-diff/v3/typed"
	"sigs.k8s.io/structured-merge-diff/v3/value"
)

type mergeTestCase struct {
	name         string
	rootTypeName string
	schema       typed.YAMLObject
	triplets     []mergeTriplet
}

type mergeTriplet struct {
	lhs typed.YAMLObject
	rhs typed.YAMLObject
	out typed.YAMLObject
	expectedErr string
}

var mergeCases = []mergeTestCase{{
	name:         "simple pair",
	rootTypeName: "stringPair",
	schema: `types:
- name: stringPair
  map:
    fields:
    - name: key
      type:
        scalar: string
    - name: value
      type:
        namedType: __untyped_atomic_
- name: __untyped_atomic_
  scalar: untyped
  list:
    elementType:
      namedType: __untyped_atomic_
    elementRelationship: atomic
  map:
    elementType:
      namedType: __untyped_atomic_
    elementRelationship: atomic
`,
	triplets: []mergeTriplet{{
		lhs: `{"key":"foo","value":{}}`,
		rhs: `{"key":"foo","value":1}`,
		out: `{"key":"foo","value":1}`,
	}, {
		lhs: `{"key":"foo","value":{}}`,
		rhs: `{"key":"foo","value":1}`,
		out: `{"key":"foo","value":1}`,
	}, {
		lhs: `{"key":"foo","value":1}`,
		rhs: `{"key":"foo","value":{}}`,
		out: `{"key":"foo","value":{}}`,
	}, {
		lhs: `{"key":"foo","value":null}`,
		rhs: `{"key":"foo","value":{}}`,
		out: `{"key":"foo","value":{}}`,
	}, {
		lhs: `{"key":"foo"}`,
		rhs: `{"value":true}`,
		out: `{"key":"foo","value":true}`,
	}},
}, {
	name:         "null/empty map",
	rootTypeName: "nestedMap",
	schema: `types:
- name: nestedMap
  map:
    fields:
    - name: inner
      type:
        map:
          elementType:
            namedType: __untyped_atomic_
- name: __untyped_atomic_
  scalar: untyped
  list:
    elementType:
      namedType: __untyped_atomic_
    elementRelationship: atomic
  map:
    elementType:
      namedType: __untyped_atomic_
    elementRelationship: atomic
`,
	triplets: []mergeTriplet{{
		lhs: `{}`,
		rhs: `{"inner":{}}`,
		out: `{"inner":{}}`,
	}, {
		lhs: `{}`,
		rhs: `{"inner":null}`,
		expectedErr: "null not allowed in applied object",
	}, {
		lhs: `{"inner":null}`,
		rhs: `{"inner":{}}`,
		out: `{"inner":{}}`,
	}, {
		lhs: `{"inner":{}}`,
		rhs: `{"inner":null}`,
		expectedErr: "null not allowed in applied object",
	}, {
		lhs: `{"inner":{}}`,
		rhs: `{"inner":{}}`,
		out: `{"inner":{}}`,
	}},
}, {
	name:         "null/empty struct",
	rootTypeName: "nestedStruct",
	schema: `types:
- name: nestedStruct
  map:
    fields:
    - name: inner
      type:
        map:
          fields:
          - name: value
            type:
              namedType: __untyped_atomic_
- name: __untyped_atomic_
  scalar: untyped
  list:
    elementType:
      namedType: __untyped_atomic_
    elementRelationship: atomic
  map:
    elementType:
      namedType: __untyped_atomic_
    elementRelationship: atomic
`,
	triplets: []mergeTriplet{{
		lhs: `{}`,
		rhs: `{"inner":{}}`,
		out: `{"inner":{}}`,
	}, {
		lhs: `{}`,
		rhs: `{"inner":null}`,
		expectedErr: "null not allowed in applied object",
	}, {
		lhs: `{"inner":null}`,
		rhs: `{"inner":{}}`,
		out: `{"inner":{}}`,
	}, {
		lhs: `{"inner":{}}`,
		rhs: `{"inner":null}`,
		expectedErr: "null not allowed in applied object",
	}, {
		lhs: `{"inner":{}}`,
		rhs: `{"inner":{}}`,
		out: `{"inner":{}}`,
	}},
}, {
	name:         "null/empty list",
	rootTypeName: "nestedList",
	schema: `types:
- name: nestedList
  map:
    fields:
    - name: inner
      type:
        list:
          elementType:
            namedType: __untyped_atomic_
          elementRelationship: atomic
- name: __untyped_atomic_
  scalar: untyped
  list:
    elementType:
      namedType: __untyped_atomic_
    elementRelationship: atomic
  map:
    elementType:
      namedType: __untyped_atomic_
    elementRelationship: atomic
`,
	triplets: []mergeTriplet{{
		lhs: `{}`,
		rhs: `{"inner":[]}`,
		out: `{"inner":[]}`,
	}, {
		lhs: `{}`,
		rhs: `{"inner":null}`,
		expectedErr: "null not allowed in applied object",
	}, {
		lhs: `{"inner":null}`,
		rhs: `{"inner":[]}`,
		out: `{"inner":[]}`,
	}, {
		lhs: `{"inner":[]}`,
		rhs: `{"inner":null}`,
		expectedErr: "null not allowed in applied object",
	}, {
		lhs: `{"inner":[]}`,
		rhs: `{"inner":[]}`,
		out: `{"inner":[]}`,
	}},
}, {
	name:         "struct grab bag",
	rootTypeName: "myStruct",
	schema: `types:
- name: myStruct
  map:
    fields:
    - name: numeric
      type:
        scalar: numeric
    - name: string
      type:
        scalar: string
    - name: bool
      type:
        scalar: boolean
    - name: setStr
      type:
        list:
          elementType:
            scalar: string
          elementRelationship: associative
    - name: setBool
      type:
        list:
          elementType:
            scalar: boolean
          elementRelationship: associative
    - name: setNumeric
      type:
        list:
          elementType:
            scalar: numeric
          elementRelationship: associative
`,
	triplets: []mergeTriplet{{
		lhs: `{"numeric":1}`,
		rhs: `{"numeric":3.14159}`,
		out: `{"numeric":3.14159}`,
	}, {
		lhs: `{"numeric":3.14159}`,
		rhs: `{"numeric":1}`,
		out: `{"numeric":1}`,
	}, {
		lhs: `{"string":"aoeu"}`,
		rhs: `{"bool":true}`,
		out: `{"string":"aoeu","bool":true}`,
	}, {
		lhs: `{"setStr":["a","b","c"]}`,
		rhs: `{"setStr":["a","b"]}`,
		out: `{"setStr":["a","b","c"]}`,
	}, {
		lhs: `{"setStr":["a","b"]}`,
		rhs: `{"setStr":["a","b","c"]}`,
		out: `{"setStr":["a","b","c"]}`,
	}, {
		lhs: `{"setStr":["a","b","c"]}`,
		rhs: `{"setStr":[]}`,
		out: `{"setStr":["a","b","c"]}`,
	}, {
		lhs: `{"setStr":[]}`,
		rhs: `{"setStr":["a","b","c"]}`,
		out: `{"setStr":["a","b","c"]}`,
	}, {
		lhs: `{"setBool":[true]}`,
		rhs: `{"setBool":[false]}`,
		out: `{"setBool":[true,false]}`,
	}, {
		lhs:`{"setNumeric":[1,2,3.14159]}`,
		rhs: `{"setNumeric":[1,2,3]}`,
		// KNOWN BUG: this order is wrong
		out: `{"setNumeric":[1,2,3.14159,3]}`,
	}},
}, {
	name:         "associative list",
	rootTypeName: "myRoot",
	schema: `types:
- name: myRoot
  map:
    fields:
    - name: list
      type:
        namedType: myList
    - name: atomicList
      type:
        namedType: mySequence
- name: myList
  list:
    elementType:
      namedType: myElement
    elementRelationship: associative
    keys:
    - key
    - id
- name: mySequence
  list:
    elementType:
      scalar: string
    elementRelationship: atomic
- name: myElement
  map:
    fields:
    - name: key
      type:
        scalar: string
    - name: id
      type:
        scalar: numeric
    - name: value
      type:
        namedType: myValue
    - name: bv
      type:
        scalar: boolean
    - name: nv
      type:
        scalar: numeric
- name: myValue
  map:
    elementType:
      scalar: string
`,
	triplets: []mergeTriplet{{
		lhs: `{"list":[{"key":"a","id":1,"value":{"a":"a"}}]}`,
		rhs: `{"list":[{"key":"a","id":1,"value":{"a":"a"}}]}`,
		out: `{"list":[{"key":"a","id":1,"value":{"a":"a"}}]}`,
	}, {
		lhs: `{"list":[{"key":"a","id":1,"value":{"a":"a"}}]}`,
		rhs: `{"list":[{"key":"a","id":2,"value":{"a":"a"}}]}`,
		out: `{"list":[{"key":"a","id":1,"value":{"a":"a"}},{"key":"a","id":2,"value":{"a":"a"}}]}`,
	}, {
		lhs: `{"list":[{"key":"a","id":1},{"key":"b","id":1}]}`,
		rhs: `{"list":[{"key":"a","id":1},{"key":"a","id":2}]}`,
		out: `{"list":[{"key":"a","id":1},{"key":"b","id":1},{"key":"a","id":2}]}`,
	}, {
		lhs: `{"atomicList":["a","a","a"]}`,
		rhs: `{"atomicList":null}`,
		expectedErr: "null not allowed in applied object",
	}, {
		lhs: `{"atomicList":["a","b","c"]}`,
		rhs: `{"atomicList":[]}`,
		out: `{"atomicList":[]}`,
	}, {
		lhs: `{"atomicList":["a","a","a"]}`,
		rhs: `{"atomicList":["a","a"]}`,
		out: `{"atomicList":["a","a"]}`,
	}},
}}

func (tt mergeTestCase) test(t *testing.T) {
	parser, err := typed.NewParser(tt.schema)
	if err != nil {
		t.Fatalf("failed to create schema: %v", err)
	}

	for i, triplet := range tt.triplets {
		triplet := triplet
		t.Run(fmt.Sprintf("%v-valid-%v", tt.name, i), func(t *testing.T) {
			t.Parallel()
			pt := parser.Type(tt.rootTypeName)

			lhs, err := pt.FromYAML(triplet.lhs)
			if err != nil {
				t.Fatalf("unable to parser/validate lhs yaml: %v\n%v", err, triplet.lhs)
			}

			rhs, err := pt.FromYAML(triplet.rhs)
			if err != nil {
				t.Fatalf("unable to parser/validate rhs yaml: %v\n%v", err, triplet.rhs)
			}

			out, err := pt.FromYAML(triplet.out)
			if err != nil {
				t.Fatalf("unable to parser/validate out yaml: %v\n%v", err, triplet.out)
			}

			got, err := lhs.Merge(rhs)

			if triplet.expectedErr != "" {
				if err == nil {
					t.Fatalf("expected error containing '%s' but got no error", triplet.expectedErr)
				}
				if !strings.Contains(err.Error(), triplet.expectedErr) {
					t.Fatalf("expected error containing '%s' but got %s", triplet.expectedErr, err.Error())
				}
				return
			}

			if err != nil {
				t.Errorf("got validation errors: %v", err)
			} else {
				if !value.Equals(got.AsValue(), out.AsValue()) {
					t.Errorf("Expected\n%v\nbut got\n%v\n",
						value.ToString(out.AsValue()), value.ToString(got.AsValue()),
					)
				}
			}
		})
	}
}

func TestMerge(t *testing.T) {
	for _, tt := range mergeCases {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			tt.test(t)
		})
	}
}
