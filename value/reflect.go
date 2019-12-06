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
	"fmt"
	"reflect"
	"strings"
	"sync"
)

var reflectPool = sync.Pool{
	New: func() interface{} {
		return &reflectValue{}
	},
}

func Reflect(value interface{}) (Value, error) {
	return wrap(reflect.ValueOf(value))
}

func MustReflect(value interface{}) Value {
	v, err := Reflect(value)
	if err != nil {
		panic(err)
	}
	return v
}

func wrap(value reflect.Value) (Value, error) {
	value = deref(value)
	if isCustomConvertable(value) {
		return toUnstructured(value)
	}
	val := reflectPool.Get().(*reflectValue)
	val.Value = value
	return Value(val), nil
}

func mustWrap(value reflect.Value) Value {
	v, err := wrap(value)
	if err != nil {
		panic(err)
	}
	return v
}

var marshalerType = reflect.TypeOf(new(json.Marshaler)).Elem()

func isCustomConvertable(val reflect.Value) bool{
	if ! val.IsValid() {
		return false
	}
	// TODO: consider corner cases (typerefs with unmarshaller interface, etc...)
	switch val.Kind() {
	case reflect.Ptr:
		return val.Type().Implements(marshalerType)
	default:
		return reflect.PtrTo(val.Type()).Implements(marshalerType)
	}
}

func toUnstructured(val reflect.Value) (Value, error) {
	// TODO: round tripping through unstructured is expensive, can we avoid for both custom conversion and merging structured with unstructured?
	data, err := json.Marshal(val.Interface())
	if err != nil {
		return nil, fmt.Errorf("error encoding %v to json: %v", val, err)
	}
	wrappedResult := struct{Value interface{}}{}
	wrappedData := fmt.Sprintf("{\"Value\": %s}", data)
	err = json.Unmarshal([]byte(wrappedData), &wrappedResult)
	if err != nil {
		return nil, fmt.Errorf("error decoding %v from json: %v", data, err)
	}
	return  NewValueInterface(wrappedResult.Value), nil
}

type reflectValue struct {
	Value reflect.Value
}

func (r reflectValue) IsMap() bool {
	return r.isKind(reflect.Map, reflect.Struct)
}

func (r reflectValue) IsList() bool {
	return r.isKind(reflect.Slice, reflect.Array)
}

func (r reflectValue) IsBool() bool {
	return r.isKind(reflect.Bool)
}

func (r reflectValue) IsInt() bool {
	// This feels wrong. Very wrong.
	return r.isKind(reflect.Int, reflect.Int64, reflect.Int32, reflect.Int16, reflect.Int8, reflect.Uint64, reflect.Uint, reflect.Uint32, reflect.Uint16, reflect.Uint8)
}

func (r reflectValue) IsFloat() bool {
	return r.isKind(reflect.Float64, reflect.Float32)
}

func (r reflectValue) IsString() bool {
	return r.isKind(reflect.String)
}

func (r reflectValue) IsNull() bool {
	return safeIsNil(r.Value)
}

func (r reflectValue) isKind(kinds ...reflect.Kind) bool {
	kind := r.Value.Kind()
	for _, k := range kinds {
		if kind == k {
			return true
		}
	}
	return false
}

// TODO find a cleaner way to avoid panics from reflect.IsNil()
func safeIsNil(v reflect.Value) bool {
	k := v.Kind()
	switch k {
	case reflect.Chan, reflect.Func, reflect.Map, reflect.Ptr, reflect.UnsafePointer, reflect.Interface, reflect.Slice:
		return v.IsNil()
	}
	return false
}

func (r reflectValue) Map() Map {
	val := r.Value
	switch val.Kind() {
	case reflect.Struct:
		return reflectStruct{Value: r.Value}
	case reflect.Map:
		return reflectMap{Value: r.Value}
	default:
		panic("value is not a map or struct")
	}
}

func (r *reflectValue) Recycle() {
	reflectPool.Put(r)
}

func (r reflectValue) List() List {
	if r.IsList() {
		return ReflectList{r.Value}
	}
	panic("value is not a list")
}

func (r reflectValue) Bool() bool {
	if r.IsBool() {
		return r.Value.Bool()
	}
	panic("value is not a bool")
}

func (r reflectValue) Int() int64 {
	// TODO: What about reflect.Value.Uint?
	if r.IsInt() {
		return r.Value.Int()
	}
	panic("value is not an int")
}

func (r reflectValue) Float() float64 {
	if r.IsFloat() {
		return r.Value.Float()
	}
	panic("value is not a float")
}

func (r reflectValue) String() string {
	if r.IsString() {
		return r.Value.String()
	}
	panic("value is not a string")
}

func (r reflectValue) Interface() interface{} {
	// In order to be mergable with unstructured, must return unstructured here
	v, err := toUnstructured(r.Value)
	if err != nil {
		panic("unable to convert to unstructured via json round-trip")
	}
	return v.Interface()
}

type reflectMap struct {
	Value reflect.Value
}

func (r reflectMap) Length() int {
	val := r.Value
	return val.Len()
}

func (r reflectMap) Get(key string) (Value, bool) {
	var val reflect.Value
	// TODO: this only works for strings and string alias key types
	val = r.Value.MapIndex(r.toMapKey(key))
	if !val.IsValid() {
		return nil, false
	}
	return mustWrap(val), val != reflect.Value{}
}

func (r reflectMap) Set(key string, val Value) {
	r.Value.SetMapIndex(r.toMapKey(key), reflect.ValueOf(val.Interface()))
}

func (r reflectMap) Delete(key string) {
	val := r.Value
	val.SetMapIndex(r.toMapKey(key), reflect.Value{})
}

func (r reflectMap) toMapKey(key string) reflect.Value {
	val := r.Value
	return reflect.ValueOf(key).Convert(val.Type().Key())
}

func (r reflectMap) Iterate(fn func(string, Value) bool) bool {
	val := r.Value
	iter := val.MapRange()
	for iter.Next() {
		next := iter.Value()
		if !next.IsValid() {
			continue
		}
		mapVal := mustWrap(next)
		if !fn(iter.Key().String(), mapVal) {
			mapVal.Recycle()
			return false
		}
	}
	return true
}

func (r reflectMap) Equals(m Map) bool {
	// TODO use reflect.DeepEqual
	return MapCompare(r, m) == 0
}

type reflectStruct struct {
	Value reflect.Value
}

func (r reflectStruct) findJsonNameField(jsonName string) (reflect.Value, bool) {
	// Try to lookup by the expected go field name first, since it's fast
	// TODO: this does not work for names like 'uid' (jsonName: uid, goName: UID)
	goName := strings.Title(jsonName)
	if field, ok := r.Value.Type().FieldByName(goName); ok && lookupJsonName(field) == jsonName {
		return r.Value.FieldByIndex(field.Index), true
	}

	// If the first lookup fails, fallback to a scan of all fields for one with a matching json tag
	var fieldVal reflect.Value
	found := false
	walkStructValues(r.Value, func(s string, value reflect.Value) bool {
		if jsonName == s {
			fieldVal = value
			found = true
			return false
		}
		return true
	})
	return fieldVal, found

}

func (r reflectStruct) Length() int {
	i := 0
	r.Iterate(func(s string, value Value) bool {
		i++
		return true
	})
	return i
}

func (r reflectStruct) Get(key string) (Value, bool) {
	if val, ok := r.findJsonNameField(key); ok {
		return mustWrap(val), true
	}
	// TODO: decide how to handle invalid keys
	return MustReflect(nil), false
}

func (r reflectStruct) Set(key string, val Value) {
	if val, ok := r.findJsonNameField(key); ok {
		val.Set(val)
	}
	// TODO: decide how to handle invalid keys
}

func (r reflectStruct) Delete(key string) {
	if val, ok := r.findJsonNameField(key); ok {
		val.Set(reflect.Value{})
	}
	// TODO: decide how to handle invalid keys
}

func (r reflectStruct) Iterate(fn func(string, Value) bool) bool {
	return walkStructValues(r.Value, func(s string, value reflect.Value) bool {
		v := mustWrap(value)
		defer v.Recycle()
		return fn(s, v)
	})
}

func walkStructValues(val reflect.Value, fn func(string, reflect.Value) bool) bool {
	for i := 0; i < val.NumField(); i++ {
		field := val.Type().Field(i)
		fieldVal := val.FieldByIndex(field.Index)
		if isInline(field) {
			if ok := walkStructValues(fieldVal, fn); !ok {
				return false
			}
		} else if isOmitempty(field) && (safeIsNil(fieldVal) || isEmptyValue(fieldVal)) {
			// skip it
		} else {
			if !fn(lookupJsonName(field), val.Field(i)) {
				return false
			}
		}
	}
	return true
}

func (r reflectStruct) Equals(m Map) bool {
	// TODO use reflect.DeepEqual
	return MapCompare(r, m) == 0
}

type ReflectList struct {
	Value reflect.Value
}

func (r ReflectList) Length() int {
	val := r.Value
	return val.Len()
}

func (r ReflectList) At(i int) Value {
	val := r.Value
	return mustWrap(val.Index(i))
}

func deref(val reflect.Value) reflect.Value {
	kind := val.Kind()
	if kind == reflect.Interface || kind == reflect.Ptr {
		return val.Elem()
	}
	return val
}

func lookupJsonName(f reflect.StructField) string {
	tag := f.Tag.Get("json")
	if tag == "-"  {
		return f.Name
	}
	name, _ := parseTag(tag)
	if name == "" {
		return f.Name
	}
	return name
}

func isInline(f reflect.StructField) bool {
	return hasOpt(f, "inline")
}

func isOmitempty(f reflect.StructField) bool {
	return hasOpt(f, "omitempty")
}

func hasOpt(f reflect.StructField, opt string) bool {
	tag := f.Tag.Get("json")
	if tag == "-"  {
		return false
	}
	_, opts := parseTag(tag)
	return opts.Contains(opt)
}

// Copied from https://golang.org/src/encoding/json/encode.go

func isEmptyValue(v reflect.Value) bool {
	switch v.Kind() {
	case reflect.Array, reflect.Map, reflect.Slice, reflect.String:
		return v.Len() == 0
	case reflect.Bool:
		return !v.Bool()
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return v.Int() == 0
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
		return v.Uint() == 0
	case reflect.Float32, reflect.Float64:
		return v.Float() == 0
	case reflect.Interface, reflect.Ptr:
		return v.IsNil()
	}
	return false
}

type tagOptions string

// parseTag splits a struct field's json tag into its name and
// comma-separated options.
func parseTag(tag string) (string, tagOptions) {
	if idx := strings.Index(tag, ","); idx != -1 {
		return tag[:idx], tagOptions(tag[idx+1:])
	}
	return tag, tagOptions("")
}

// Contains reports whether a comma-separated list of options
// contains a particular substr flag. substr must be surrounded by a
// string boundary or commas.
func (o tagOptions) Contains(optionName string) bool {
	if len(o) == 0 {
		return false
	}
	s := string(o)
	for s != "" {
		var next string
		i := strings.Index(s, ",")
		if i >= 0 {
			s, next = s[:i], s[i+1:]
		}
		if s == optionName {
			return true
		}
		s = next
	}
	return false
}