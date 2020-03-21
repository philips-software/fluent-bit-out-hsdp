package hsdp

import (
	"encoding/json"
	"unicode"
)

// CamelCaseToUnderscore converts from camel case form to underscore separated form.
// Ex.: MyFunc => my_func
// From govalidator MIT Copyright (c) 2014 Alex Saskevich
func CamelCaseToUnderscore(str string) string {
	var output []rune
	var segment []rune
	for _, r := range str {

		// not treat number as separate segment
		if !unicode.IsLower(r) && string(r) != "_" && !unicode.IsNumber(r) {
			output = addSegment(output, segment)
			segment = nil
		}
		segment = append(segment, unicode.ToLower(r))
	}
	output = addSegment(output, segment)
	return string(output)
}

func addSegment(inrune, segment []rune) []rune {
	if len(segment) == 0 {
		return inrune
	}
	if len(inrune) != 0 {
		inrune = append(inrune, '_')
	}
	inrune = append(inrune, segment...)
	return inrune
}

// https://github.com/go-testfixtures/testfixtures/blob/2a311be25d1bcba689767eeb7b8f1fd67b9343f5/json.go
type jsonArray []interface{}

func (a jsonArray) Value() ([]byte, error) {
	return json.Marshal(a)
}

type jsonMap map[string]interface{}

func (m jsonMap) Value() ([]byte, error) {
	return json.Marshal(m)
}

// Go refuses to convert map[interface{}]interface{} to JSON because JSON only support string keys
// So it's necessary to recursively convert all map[interface]interface{} to map[string]interface{}
func recursiveToJSON(v interface{}) (r interface{}) {
	switch v := v.(type) {
	case []interface{}:
		for i, e := range v {
			v[i] = recursiveToJSON(e)
		}
		r = jsonArray(v)
	case map[interface{}]interface{}:
		newMap := make(map[string]interface{}, len(v))
		for k, e := range v {
			newMap[k.(string)] = recursiveToJSON(e)
		}
		r = jsonMap(newMap)
	case []byte:
		// Prevent base64 encoding
		r = string(v)
	default:
		r = v
	}
	return
}
