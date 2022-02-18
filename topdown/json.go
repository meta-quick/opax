// Copyright 2019 The OPA Authors.  All rights reserved.
// Use of this source code is governed by an Apache2
// license that can be found in the LICENSE file.

package topdown

import (
	"encoding/json"
	"fmt"
	"github.com/bytedance/sonic"
	"github.com/meta-quick/mask/jsonmask"
	"github.com/meta-quick/mask/types"
	"github.com/meta-quick/opa/ast"
	"github.com/meta-quick/opa/topdown/builtins"
	"strconv"
	"strings"
	"sync"
)

var (
	shuffle_mutex sync.Mutex
	shuffleModel = make(map[string]*interface{})
)

func ShuffleModelAddString(key string, value string) {
	var v interface{}
	if err := sonic.Unmarshal([]byte(value), &v); err == nil {
		ShuffleModelAdd(key, &v)
	}
}

func ShuffleModelAdd(key string, value *interface{}) {
	shuffle_mutex.Lock()
	defer shuffle_mutex.Unlock()

	val := (*value).(map[string]interface{})
	//Compile shuffle
    var shuffle	= make(map[string]interface{})
	for k, v := range val {
		if k == "shuffle" {
			for kk, vv := range v.(map[string]interface{}) {
				for kkk, vvv := range vv.(map[string]interface{}) {
					switch tt := vvv.(type) {
					case []interface{}:
						handle := jsonmask.ProcessHandle{
							Fn: kkk,
						}
                        var args = []string{}
						for _,vvvv := range tt{
                            args = append(args,vvvv.(string))
						}
						handle.Args = args
						shuffle[kk] = handle
					}
				}
			}
		}
	}
	val["shuffle"] = shuffle
	shuffleModel[key] = value
}

func ShuffleModelGet(key string) *interface{} {
	if v, ok := shuffleModel[key]; ok {
		return v
	} else {
		return nil
	}
}

func ShuffleModelDel(key string) {
	shuffle_mutex.Lock()
	defer shuffle_mutex.Unlock()
	if _, ok := shuffleModel[key]; ok {
		delete(shuffleModel, key)
	}
}

func builtinJSONRemove(_ BuiltinContext, operands []*ast.Term, iter func(*ast.Term) error) error {

	// Expect an object and a string or array/set of strings
	_, err := builtins.ObjectOperand(operands[0].Value, 1)
	if err != nil {
		return err
	}

	// Build a list of json pointers to remove
	paths, err := getJSONPaths(operands[1].Value)
	if err != nil {
		return err
	}

	newObj, err := jsonRemove(operands[0], ast.NewTerm(pathsToObject(paths)))
	if err != nil {
		return err
	}

	if newObj == nil {
		return nil
	}

	return iter(newObj)
}

// jsonRemove returns a new term that is the result of walking
// through a and omitting removing any values that are in b but
// have ast.Null values (ie leaf nodes for b).
func jsonRemove(a *ast.Term, b *ast.Term) (*ast.Term, error) {
	if b == nil {
		// The paths diverged, return a
		return a, nil
	}

	var bObj ast.Object
	switch bValue := b.Value.(type) {
	case ast.Object:
		bObj = bValue
	case ast.Null:
		// Means we hit a leaf node on "b", dont add the value for a
		return nil, nil
	default:
		// The paths diverged, return a
		return a, nil
	}

	switch aValue := a.Value.(type) {
	case ast.String, ast.Number, ast.Boolean, ast.Null:
		return a, nil
	case ast.Object:
		newObj := ast.NewObject()
		err := aValue.Iter(func(k *ast.Term, v *ast.Term) error {
			// recurse and add the diff of sub objects as needed
			diffValue, err := jsonRemove(v, bObj.Get(k))
			if err != nil || diffValue == nil {
				return err
			}
			newObj.Insert(k, diffValue)
			return nil
		})
		if err != nil {
			return nil, err
		}
		return ast.NewTerm(newObj), nil
	case ast.Set:
		newSet := ast.NewSet()
		err := aValue.Iter(func(v *ast.Term) error {
			// recurse and add the diff of sub objects as needed
			diffValue, err := jsonRemove(v, bObj.Get(v))
			if err != nil || diffValue == nil {
				return err
			}
			newSet.Add(diffValue)
			return nil
		})
		if err != nil {
			return nil, err
		}
		return ast.NewTerm(newSet), nil
	case *ast.Array:
		// When indexes are removed we shift left to close empty spots in the array
		// as per the JSON patch spec.
		newArray := ast.NewArray()
		for i := 0; i < aValue.Len(); i++ {
			v := aValue.Elem(i)
			// recurse and add the diff of sub objects as needed
			// Note: Keys in b will be strings for the index, eg path /a/1/b => {"a": {"1": {"b": null}}}
			diffValue, err := jsonRemove(v, bObj.Get(ast.StringTerm(strconv.Itoa(i))))
			if err != nil {
				return nil, err
			}
			if diffValue != nil {
				newArray = newArray.Append(diffValue)
			}
		}
		return ast.NewTerm(newArray), nil
	default:
		return nil, fmt.Errorf("invalid value type %T", a)
	}
}

func builtinJSONFilter(_ BuiltinContext, operands []*ast.Term, iter func(*ast.Term) error) error {

	// Ensure we have the right parameters, expect an object and a string or array/set of strings
	obj, err := builtins.ObjectOperand(operands[0].Value, 1)
	if err != nil {
		return err
	}

	// Build a list of filter strings
	filters, err := getJSONPaths(operands[1].Value)
	if err != nil {
		return err
	}

	// Actually do the filtering
	filterObj := pathsToObject(filters)
	r, err := obj.Filter(filterObj)
	if err != nil {
		return err
	}

	return iter(ast.NewTerm(r))
}

func getJSONPaths(operand ast.Value) ([]ast.Ref, error) {
	var paths []ast.Ref

	switch v := operand.(type) {
	case *ast.Array:
		for i := 0; i < v.Len(); i++ {
			filter, err := parsePath(v.Elem(i))
			if err != nil {
				return nil, err
			}
			paths = append(paths, filter)
		}
	case ast.Set:
		err := v.Iter(func(f *ast.Term) error {
			filter, err := parsePath(f)
			if err != nil {
				return err
			}
			paths = append(paths, filter)
			return nil
		})
		if err != nil {
			return nil, err
		}
	default:
		return nil, builtins.NewOperandTypeErr(2, v, "set", "array")
	}

	return paths, nil
}

func parsePath(path *ast.Term) (ast.Ref, error) {
	// paths can either be a `/` separated json path or
	// an array or set of values
	var pathSegments ast.Ref
	switch p := path.Value.(type) {
	case ast.String:
		if p == "" {
			return ast.Ref{}, nil
		}
		parts := strings.Split(strings.TrimLeft(string(p), "/"), "/")
		for _, part := range parts {
			part = strings.ReplaceAll(strings.ReplaceAll(part, "~1", "/"), "~0", "~")
			pathSegments = append(pathSegments, ast.StringTerm(part))
		}
	case *ast.Array:
		p.Foreach(func(term *ast.Term) {
			pathSegments = append(pathSegments, term)
		})
	default:
		return nil, builtins.NewOperandErr(2, "must be one of {set, array} containing string paths or array of path segments but got %v", ast.TypeName(p))
	}

	return pathSegments, nil
}

func extendPath(path ast.Ref,count int) ([]ast.Ref, error) {
	if len(path) == 0 {
		return []ast.Ref{path}, nil
	}
    _result := []ast.Ref{
	}

	for i := 0; i < len(path); i++ {
		switch v := path[i].Value.(type) {
		case ast.Number:
			if( len(_result) == 0 ){
				_result = append(_result, ast.Ref{path[i]})
			} else {
				for m := 0; m < len(_result); m++ {
					_result[m] = append(_result[m], path[i])
				}
			}
		case ast.String:
			if strings.Contains(string(v),":")  {
				parts := strings.Split(string(v),":")
				start := -1
				end := -1
				if len(parts) == 2 {
					if val, err := strconv.Atoi(parts[0]); err == nil {
						start = val
					}
					if val, err := strconv.Atoi(parts[1]); err == nil {
						end = val
					}
				}

				if len(parts) == 1 {
					if strings.HasPrefix(string(v), ":") {
						if val, err := strconv.Atoi(parts[0]); err == nil {
							end = val
						}
					} else {
						if val, err := strconv.Atoi(parts[0]); err == nil {
							start = val
						}
					}
				}

				if start > -1  && end > -1 {
					__results := []ast.Ref{}
					for m := start; m < end; m++ {
						for j := 0; j < len(_result); j++ {
							tt := append(_result[j].Copy(), ast.IntNumberTerm(m))
							__results = append(__results, tt)
						}
					}
					_result = __results
				} else if start > -1 {
					__results := []ast.Ref{}
					for m := start; m < start + count; m++ {
						for j := 0; j < len(_result); j++ {
							tt := append(_result[j].Copy(), ast.IntNumberTerm(m))
							__results = append(__results, tt)
						}
					}
					_result = __results
				} else if end > -1 {
					__results := []ast.Ref{}
					for m := 0; m < end; m++ {
						for j := 0; j < len(_result); j++ {
							tt := append(_result[j].Copy(), ast.IntNumberTerm(m))
							__results = append(__results, tt)
						}
					}
					_result = __results
				} else {
					__results := []ast.Ref{}
					for m := 0; m < count; m++ {
						for j := 0; j < len(_result); j++ {
							tt := append(_result[j].Copy(), ast.IntNumberTerm(m))
							__results = append(__results, tt)
						}
					}
					_result = __results
				}
			} else {
				if( len(_result) == 0 ){
					_result = append(_result, ast.Ref{path[i]})
				} else {
					for m := 0; m < len(_result); m++ {
						_result[m] = append(_result[m], path[i])
					}
				}
			}
		}
	}

	return _result, nil
}

func pathsToObject(paths []ast.Ref) ast.Object {

	root := ast.NewObject()

	for _, path := range paths {
		node := root
		var done bool

		for i := 0; i < len(path)-1 && !done; i++ {

			k := path[i]
			child := node.Get(k)

			if child == nil {
				obj := ast.NewObject()
				node.Insert(k, ast.NewTerm(obj))
				node = obj
				continue
			}

			switch v := child.Value.(type) {
			case ast.Null:
				done = true
			case ast.Object:
				node = v
			default:
				panic("unreachable")
			}
		}

		if !done {
			node.Insert(path[len(path)-1], ast.NullTerm())
		}
	}

	return root
}

type IndexRange struct {
	Start int
	End   int
}

// toIndex tries to convert path elements (that may be strings) into indices into
// an array.
func toIndex(arr *ast.Array, term *ast.Term) (*IndexRange, error) {
	i := 0
	var ok bool
	switch v := term.Value.(type) {
	case ast.Number:
		if i, ok = v.Int(); !ok {
			return nil, fmt.Errorf("Invalid number type for indexing")
		}
	case ast.String:
		if strings.Contains(string(v),":")  {
			parts := strings.Split(string(v),":")
			index := IndexRange{
				Start: 0,
				End:   arr.Len(),
			}
			if len(parts) == 2 {
				if start, err := strconv.Atoi(parts[0]); err == nil {
					index.Start = start
				}
				if end, err := strconv.Atoi(parts[1]); err == nil {
					index.End = end
				}
			}

			if len(parts) == 1 {
				if strings.HasPrefix(string(v), ":") {
					if end, err := strconv.Atoi(parts[0]); err == nil {
						index.End = end
					}
				} else {
					if start, err := strconv.Atoi(parts[0]); err == nil {
						index.Start = start
					}
				}
			}

			if index.Start > index.End {
				index.Start, index.End = index.End, index.Start
			}

			if(index.Start < 0) {
				index.Start = 0
			}
			if index.End > arr.Len() {
				index.End = arr.Len()
			}

			return &index, nil
		}
		num := ast.Number(v)
		if i, ok = num.Int(); !ok {
			return nil, fmt.Errorf("Invalid string for indexing")
		}
		if v != "0" && strings.HasPrefix(string(v), "0") {
			return nil, fmt.Errorf("Leading zeros are not allowed in JSON paths")
		}
	default:
		return nil, fmt.Errorf("Invalid type for indexing")
	}
	if i >= arr.Len() {
		i = arr.Len() -1
	}

	if i < 0 {
		i = arr.Len() + i
	}

	if i < 0 {
		i = 0
	}

	index := IndexRange{
		Start: i,
		End:   i+1,
	}

	return &index, nil
}

// patchWorkerris a worker that modifies a direct child of a term located
// at the given key.  It returns the new term, and optionally a result that
// is passed back to the caller.
type patchWorker = func(parent, key *ast.Term) (updated *ast.Term, result *ast.Term, expanded_path []ast.Ref)

func jsonPatchTraverse(
	target *ast.Term,
	path ast.Ref,
	worker patchWorker,
) (*ast.Term, *ast.Term,[]ast.Ref) {
	if len(path) < 1 {
		return nil, nil,nil
	}

	key := path[0]
	if len(path) == 1 {
		return worker(target, key)
	}

	success := false
	var updated, result *ast.Term
	var updated_path = []ast.Ref{}
	switch parent := target.Value.(type) {
	case ast.Object:
		obj := ast.NewObject()
		parent.Foreach(func(k, v *ast.Term) {
			if k.Equal(key) {
				var _path []ast.Ref
				if v, result,_path = jsonPatchTraverse(v, path[1:], worker); v != nil {
					obj.Insert(k, v)
					for _,__path := range _path {
						tmp_path := ast.Ref{path[0]}
						for _,part := range __path {
							tmp_path = tmp_path.Append(part)
						}
						updated_path = append(updated_path,tmp_path)
					}
					success = true
				}
			} else {
				obj.Insert(k, v)
			}
		})
		updated = ast.NewTerm(obj)

	case *ast.Array:
		idx, err := toIndex(parent, key)
		if err != nil || idx == nil {
			return nil, nil, nil
		}
		arr := ast.NewArray()
		_results := ast.NewArray()
		for i := 0; i < parent.Len(); i++ {
			v := parent.Elem(i)
			if i >= idx.Start && i < idx.End {
				if v, ret,_path := jsonPatchTraverse(v, path[1:], worker); v != nil {
					arr = arr.Append(v)
                    if ret != nil {
						_results = _results.Append(ret)
					}

					for _,__path := range _path {
						tmp_path := ast.Ref{ast.IntNumberTerm(i)}
						for _,part := range __path {
							tmp_path = tmp_path.Append(part)
						}
						updated_path = append(updated_path,tmp_path)
					}
					success = true
				}
			} else {
				arr = arr.Append(v)
			}
		}
		if success {
			result = ast.NewTerm(_results)
		}
		updated = ast.NewTerm(arr)

	case ast.Set:
		set := ast.NewSet()
		parent.Foreach(func(k *ast.Term) {
			if k.Equal(key) {
				var _path  []ast.Ref
				if k, result,_path = jsonPatchTraverse(k, path[1:], worker); k != nil {
					set.Add(k)
					for _,__path := range _path {
						tmp_path := ast.Ref{path[0]}
						for _,part := range __path {
							tmp_path = tmp_path.Append(part)
						}
						updated_path = append(updated_path,tmp_path)
					}
					success = true
				}
			} else {
				set.Add(k)
			}
		})
		updated = ast.NewTerm(set)
	}

	if success {
		return updated, result,updated_path
	}

	return nil, nil, nil
}

// jsonPatchGet goes one step further than jsonPatchTraverse and returns the
// term at the location specified by the path.  It is used in functions
// where we want to read a value but not manipulate its parent: for example
// jsonPatchTest and jsonPatchCopy.
//
// Because it uses jsonPatchTraverse, it makes shallow copies of the objects
// along the path.  We could possibly add a signaling mechanism that we didn't
// make any changes to avoid this.
func jsonPatchGet(target *ast.Term, path ast.Ref) (*ast.Term,[]ast.Ref) {
	// Special case: get entire document.
	if len(path) == 0 {
		return target,nil
	}

	_, result,_path := jsonPatchTraverse(target, path, func(parent, key *ast.Term) (*ast.Term, *ast.Term,[]ast.Ref) {
		switch v := parent.Value.(type) {
		case ast.Object:
			return parent, v.Get(key),[]ast.Ref{ast.Ref{key}}
		case *ast.Array:
			idx, err := toIndex(v, key)
			if err == nil {
				__result := ast.NewArray()
				__path := []ast.Ref{}
				for i := idx.Start; i< idx.End; i++ {
					__result = __result.Append(v.Elem(i))
					tmp_path := ast.Ref{ast.IntNumberTerm(i)}
					__path = append(__path,tmp_path)
				}
				return parent, ast.NewTerm(__result),__path
			}
		case ast.Set:
			if v.Contains(key) {
				return parent, key,[]ast.Ref{ast.Ref{key}}
			}
		}
		return nil, nil,nil
	})
	return result,_path
}

func jsonPatchAdd(target *ast.Term, path ast.Ref, value *ast.Term) *ast.Term {
	// Special case: replacing root document.
	if len(path) == 0 {
		return value
	}

	target, _,_ = jsonPatchTraverse(target, path, func(parent *ast.Term, key *ast.Term) (*ast.Term, *ast.Term,[]ast.Ref) {
		switch original := parent.Value.(type) {
		case ast.Object:
			obj := ast.NewObject()
			original.Foreach(func(k, v *ast.Term) {
				obj.Insert(k, v)
			})
			obj.Insert(key, value)
			return ast.NewTerm(obj), nil,nil
		case *ast.Array:
			idx, err := toIndex(original, key)
			if err != nil || idx == nil {
				return nil, nil,nil
			}
			arr := ast.NewArray()
			for i := 0; i < idx.Start; i++ {
				arr = arr.Append(original.Elem(i))
			}

			//added range should [start,end)
			for i := idx.Start; i < idx.End; i++ {
				arr = arr.Append(value)
			}

			for i := idx.Start; i < original.Len(); i++ {
				arr = arr.Append(original.Elem(i))
			}
			return ast.NewTerm(arr), nil,nil
		case ast.Set:
			if !key.Equal(value) {
				return nil, nil,nil
			}
			set := ast.NewSet()
			original.Foreach(func(k *ast.Term) {
				set.Add(k)
			})
			set.Add(key)
			return ast.NewTerm(set), nil,nil
		}
		return nil, nil,nil
	})

	return target
}

func jsonPatchRemove(target *ast.Term, path ast.Ref) (*ast.Term, *ast.Term) {
	// Special case: replacing root document.
	if len(path) == 0 {
		return nil, nil
	}

	target, removed,_ := jsonPatchTraverse(target, path, func(parent *ast.Term, key *ast.Term) (*ast.Term, *ast.Term,[]ast.Ref) {
		var removed *ast.Term
		switch original := parent.Value.(type) {
		case ast.Object:
			obj := ast.NewObject()
			original.Foreach(func(k, v *ast.Term) {
				if k.Equal(key) {
					removed = v
				} else {
					obj.Insert(k, v)
				}
			})
			return ast.NewTerm(obj), removed,nil
		case *ast.Array:
			idx, err := toIndex(original, key)
			if err != nil || idx == nil {
				return nil, nil,nil
			}
			arr := ast.NewArray()
			for i := 0; i < idx.Start; i++ {
				arr = arr.Append(original.Elem(i))
			}
			//remove should [start,end)
			_removed := ast.NewArray()
			for i := idx.Start; i < idx.End; i++ {
				_removed = _removed.Append(original.Elem(i))
			}
			removed = ast.NewTerm(_removed)

			for i := idx.End; i < original.Len(); i++ {
				arr = arr.Append(original.Elem(i))
			}
			return ast.NewTerm(arr), removed,nil
		case ast.Set:
			set := ast.NewSet()
			original.Foreach(func(k *ast.Term) {
				if k.Equal(key) {
					removed = k
				} else {
					set.Add(k)
				}
			})
			return ast.NewTerm(set), removed,nil
		}
		return nil, nil,nil
	})

	if target != nil && removed != nil {
		return target, removed
	}

	return nil, nil
}

func jsonPatchReplace(target *ast.Term, path ast.Ref, value *ast.Term) *ast.Term {
	// Special case: replacing the whole document.
	if len(path) == 0 {
		return value
	}

	// Replace is specified as `remove` followed by `add`.
	if target, _ = jsonPatchRemove(target, path); target == nil {
		return nil
	}

	return jsonPatchAdd(target, path, value)
}

func jsonPatchMove(target *ast.Term, path ast.Ref, from ast.Ref) *ast.Term {
	// Move is specified as `remove` followed by `add`.
	target, removed := jsonPatchRemove(target, from)
	if target == nil || removed == nil {
		return nil
	}

	return jsonPatchAdd(target, path, removed)
}

func jsonPatchCopy(target *ast.Term, path ast.Ref, from ast.Ref) *ast.Term {
	value,_ := jsonPatchGet(target, from)
	if value == nil {
		return nil
	}

	return jsonPatchAdd(target, path, value)
}

func jsonPatchTest(target *ast.Term, path ast.Ref, value *ast.Term) *ast.Term {
	actual,_:= jsonPatchGet(target, path)
	if actual == nil {
		return nil
	}

	if actual.Equal(value) {
		return target
	}

	return nil
}

func builtinJSONPatch(_ BuiltinContext, operands []*ast.Term, iter func(*ast.Term) error) error {
	// JSON patch supports arrays, objects as well as values as the target.
	target := ast.NewTerm(operands[0].Value)

	// Expect an array of operations.
	operations, err := builtins.ArrayOperand(operands[1].Value, 2)
	if err != nil {
		return err
	}

	// Apply operations one by one.
	for i := 0; i < operations.Len(); i++ {
		if object, ok := operations.Elem(i).Value.(ast.Object); ok {
			getAttribute := func(attr string) (*ast.Term, error) {
				if term := object.Get(ast.StringTerm(attr)); term != nil {
					return term, nil
				}

				return nil, builtins.NewOperandErr(2, fmt.Sprintf("patch is missing '%s' attribute", attr))
			}

			getPathAttribute := func(attr string) (ast.Ref, error) {
				term, err := getAttribute(attr)
				if err != nil {
					return ast.Ref{}, err
				}
				path, err := parsePath(term)
				if err != nil {
					return ast.Ref{}, err
				}
				return path, nil
			}

			// Parse operation.
			opTerm, err := getAttribute("op")
			if err != nil {
				return err
			}
			op, ok := opTerm.Value.(ast.String)
			if !ok {
				return builtins.NewOperandErr(2, "patch attribute 'op' must be a string")
			}

			// Parse path.
			path, err := getPathAttribute("path")
			if err != nil {
				return err
			}

			switch op {
			case "add":
				value, err := getAttribute("value")
				if err != nil {
					return err
				}
				target = jsonPatchAdd(target, path, value)
			case "remove":
				target, _ = jsonPatchRemove(target, path)
			case "replace":
				value, err := getAttribute("value")
				if err != nil {
					return err
				}
				target = jsonPatchReplace(target, path, value)
			case "move":
				from, err := getPathAttribute("from")
				if err != nil {
					return err
				}
				target = jsonPatchMove(target, path, from)
			case "copy":
				from, err := getPathAttribute("from")
				if err != nil {
					return err
				}
				target = jsonPatchCopy(target, path, from)
			case "test":
				value, err := getAttribute("value")
				if err != nil {
					return err
				}
				target = jsonPatchTest(target, path, value)
			default:
				return builtins.NewOperandErr(2, "must be an array of JSON-Patch objects")
			}
		} else {
			return builtins.NewOperandErr(2, "must be an array of JSON-Patch objects")
		}

		// JSON patches should work atomically; and if one of them fails,
		// we should not try to continue.
		if target == nil {
			return nil
		}
	}

	return iter(target)
}

func builtinJSONShuffle(_ BuiltinContext, operands []*ast.Term, iter func(*ast.Term) error) error {
	// JSON patch supports arrays, objects as well as values as the target.
	target := ast.NewTerm(operands[0].Value)
	// Shuffle model namesapce.
	model := string(operands[1].Value.(ast.String))
	// Shuffle model namesapce.
	model = model + "/" + string(operands[2].Value.(ast.String))

	// Expect an array of operations.
	operations, err := builtins.ArrayOperand(operands[3].Value, 2)
	if err != nil {
		return err
	}

	// Apply operations one by one.
	for i := 0; i < operations.Len(); i++ {
		if object, ok := operations.Elem(i).Value.(ast.Object); ok {
			getAttribute := func(attr string) (*ast.Term, error) {
				if term := object.Get(ast.StringTerm(attr)); term != nil {
					return term, nil
				}

				return nil, builtins.NewOperandErr(2, fmt.Sprintf("patch is missing '%s' attribute", attr))
			}

			getPathAttribute := func(attr string) (ast.Ref, error) {
				term, err := getAttribute(attr)
				if err != nil {
					return ast.Ref{}, err
				}
				path, err := parsePath(term)
				if err != nil {
					return ast.Ref{}, err
				}
				return path, nil
			}

			// Parse operation.
			opTerm, err := getAttribute("op")
			if err != nil {
				return err
			}
			op, ok := opTerm.Value.(ast.String)
			if !ok {
				return builtins.NewOperandErr(2, "patch attribute 'op' must be a string")
			}

			// Parse path.
			path, err := getPathAttribute("path")
			if err != nil {
				return err
			}

			switch op {
			case "add":
				value, err := getAttribute("value")
				if err != nil {
					return err
				}
				target = jsonPatchAdd(target, path, value)
			case "remove":
				target, _ = jsonPatchRemove(target, path)
			case "replace":
				value, err := getAttribute("value")
				if err != nil {
					return err
				}
				target = jsonPatchReplace(target, path, value)
			case "move":
				from, err := getPathAttribute("from")
				if err != nil {
					return err
				}
				target = jsonPatchMove(target, path, from)
			case "copy":
				from, err := getPathAttribute("from")
				if err != nil {
					return err
				}
				target = jsonPatchCopy(target, path, from)
			case "test":
				value, err := getAttribute("value")
				if err != nil {
					return err
				}
				target = jsonPatchTest(target, path, value)
			default:
				return builtins.NewOperandErr(2, "must be an array of JSON-Patch objects")
			}
		} else {
			return builtins.NewOperandErr(2, "must be an array of JSON-Patch objects")
		}

		// JSON patches should work atomically; and if one of them fails,
		// we should not try to continue.
		if target == nil {
			return nil
		}
	}

	shuffle := ShuffleModelGet(model)
	if shuffle !=nil {
        // Remove denied fields
		switch v := (*shuffle).(type) {
		case map[string]interface{}:
			for key, field := range v {
				if(key == "filters"){
					switch vv := field.(type) {
					case  map[string]interface{}:
						for denied, column := range vv {
							if(denied == "denied"){
								switch columns := column.(type) {
								case []interface{}:
									for _, field := range columns {
										path,_ := parsePath(ast.StringTerm(field.(string)))
										target, _ = jsonPatchRemove(target, path)
									}
								}
							}
						}
					}
				}

				if(key == "shuffle"){
					switch shfl := field.(type) {
					case map[string]interface{}:
						for path, fn := range shfl {
							path,_ := parsePath(ast.StringTerm(path))
							origin,extpath := jsonPatchGet(target,path)
							if origin != nil {
								switch vv := origin.Value.(type) {
								case *ast.Array:
									//extpath,_ := extendPath(path,vv.Len())
									for i := 0; i < vv.Len(); i++ {
										v := vv.Elem(i)
										step,_ := ast.ValueToInterfaceX(v.Value)
										ctx := types.BuiltinContext{
											Fn: fn.(jsonmask.ProcessHandle).Fn,
											Args: fn.(jsonmask.ProcessHandle).Args,
											Current: typeCasting(step),
										}
										types.Eval(&ctx)
										newValue,err := ast.InterfaceToValue(ctx.Result)
										if err == nil {
											target = jsonPatchReplace(target, extpath[i], ast.NewTerm(newValue))
										}
									}
								default:
									step,_ := ast.ValueToInterfaceX(vv)
									ctx := types.BuiltinContext{
										Fn: fn.(jsonmask.ProcessHandle).Fn,
										Args: fn.(jsonmask.ProcessHandle).Args,
										Current: typeCasting(step),
									}
									types.Eval(&ctx)
									newValue,err := ast.InterfaceToValue(ctx.Result)
									if err == nil {
										target = jsonPatchReplace(target, path, ast.NewTerm(newValue))
									}
								}
							}
						}
					}
				}
			}
		}
	}

	return iter(target)
}

func typeCasting(d interface{}) string {
	switch c := d.(type) {
	case string:
		return c
	case int32:
		return fmt.Sprint(c)
	case int64:
		return fmt.Sprint(c)
	case float64:
		return strconv.FormatFloat(c, 'f', -1, 64)
	case json.Number:
		tt,_ := strconv.ParseFloat(c.String(),64)
		return strconv.FormatFloat(tt, 'f', -1, 64)
	default:
		return fmt.Sprint(d)
	}
}

func init() {
	RegisterBuiltinFunc(ast.JSONFilter.Name, builtinJSONFilter)
	RegisterBuiltinFunc(ast.JSONRemove.Name, builtinJSONRemove)
	RegisterBuiltinFunc(ast.JSONPatch.Name, builtinJSONPatch)
	RegisterBuiltinFunc(ast.JSONShuffle.Name, builtinJSONShuffle)
}
