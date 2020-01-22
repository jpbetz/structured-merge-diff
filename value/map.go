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
	"sort"
	"strings"
)

// Map represents a Map or go structure.
type Map interface {
	// Set changes or set the value of the given key.
	Set(key string, val Value)
	// get returns the value for the given key, if present, or (nil, false) otherwise.
	Get(key string) (Value, bool)
	// Has returns true if the key is present, or false otherwise.
	Has(key string) bool
	// Delete removes the key from the map.
	Delete(key string)
	// Equals compares the two maps, and return true if they are the same, false otherwise.
	// Implementations can use MapEquals as a general implementation for this methods.
	Equals(other Map) bool
	// Iterate runs the given function for each key/value in the
	// map. Returning false in the closure prematurely stops the
	// iteration.
	Iterate(func(key string, value Value) bool) bool
	// Range returns a MapRange for iterating over the entry in the map.
	Range() MapRange
	// Length returns the number of items in the map.
	Length() int
}

// MapRange represents a single iteration across the entry of a map.
type MapRange interface {
	// Next increments to the next entry in the range, if there is one, and returns true, or returns false if there are no more entry.
	Next() bool
	// Key returns the value of the current entry in the range. or panics if there is no current entry.
	Key() string
	// Value returns the value of the current entry in the range. or panics if there is no current entry.
	// For efficiency, Value may reuse the values returned by previous Value calls. Callers should be careful avoid holding
	// pointers to the value returned by Value() that escape the iteration loop since they become invalid once either
	// Value() or Recycle() is called.
	Value() Value
	// Recycle returns a ListRange that is no longer needed. The value returned by Item() becomes invalid once this is
	// called.
	Recycle()
}

// MapLess compares two maps lexically.
func MapLess(lhs, rhs Map) bool {
	return MapCompare(lhs, rhs) == -1
}

// MapCompare compares two maps lexically.
func MapCompare(lhs, rhs Map) int {
	lorder := make([]string, 0, lhs.Length())
	iter := lhs.Range()
	defer iter.Recycle()
	for iter.Next() {
		key := iter.Key()
		lorder = append(lorder, key)
	}
	sort.Strings(lorder)
	rorder := make([]string, 0, rhs.Length())
	iter = rhs.Range()
	defer iter.Recycle()
	for iter.Next() {
		key := iter.Key()
		rorder = append(rorder, key)
	}
	sort.Strings(rorder)

	i := 0
	for {
		if i >= len(lorder) && i >= len(rorder) {
			// Maps are the same length and all items are equal.
			return 0
		}
		if i >= len(lorder) {
			// LHS is shorter.
			return -1
		}
		if i >= len(rorder) {
			// RHS is shorter.
			return 1
		}
		if c := strings.Compare(lorder[i], rorder[i]); c != 0 {
			return c
		}
		litem, _ := lhs.Get(lorder[i])
		ritem, _ := rhs.Get(rorder[i])
		c := Compare(litem, ritem)
		if litem != nil {
			litem.Recycle()
		}
		if ritem != nil {
			ritem.Recycle()
		}
		if c != 0 {
			return c
		}
		// The items are equal; continue.
		i++
	}
}

// MapEquals returns true if lhs == rhs, false otherwise. This function
// acts on generic types and should not be used by callers, but can help
// implement Map.Equals.
// WARN: This is a naive implementation, calling lhs.Equals(rhs) is typically far more efficient.
func MapEquals(lhs, rhs Map) bool {
	if lhs.Length() != rhs.Length() {
		return false
	}
	iter := lhs.Range()
	defer iter.Recycle()
	for iter.Next() {
		k := iter.Key()
		v := iter.Value()
		vo, ok := rhs.Get(k)
		if !ok {
			return false
		}
		equal := Equals(v, vo)
		vo.Recycle()
		if !equal {
			return false
		}
	}
	return true
}
