package utils

import (
	"fmt"
	"reflect"
)

type ArrayComparisonStrategyFactory func(path string) ArrayComparisonStrategy
type ArrayComparisonStrategy func(expectedData, actualData []interface{}) error

var StrategyFactory ArrayComparisonStrategyFactory

// SubsetError is an error type used by IsSubset for tracking the path in the struct.
type SubsetError struct {
	path    []string
	message string
}

// AppendPath appends key to the existing struct path. For example, in struct member `a.Key1.Key2`, the path would be ["Key1", "Key2"]
func (e *SubsetError) AppendPath(key string) {
	if e.path == nil {
		e.path = []string{}
	}

	e.path = append(e.path, key)
}

// Error implements the error interface.
func (e *SubsetError) Error() string {
	if e.path == nil || len(e.path) == 0 {
		return e.message
	}

	path := ""
	for i := len(e.path) - 1; i >= 0; i-- {
		path = fmt.Sprintf("%s.%s", path, e.path[i])
	}

	return fmt.Sprintf("%s: %s", path, e.message)
}

// IsSubset checks to see if `expected` is a subset of `actual`. A "subset" is an object that is equivalent to
// the other object, but where map keys found in actual that are not defined in expected are ignored.
func IsSubset(expected, actual interface{}, currentPath string, strategyFactory ArrayComparisonStrategyFactory) error {
	if reflect.TypeOf(expected) != reflect.TypeOf(actual) {
		return &SubsetError{
			message: fmt.Sprintf("type mismatch: %v != %v", reflect.TypeOf(expected), reflect.TypeOf(actual)),
		}
	}

	if reflect.DeepEqual(expected, actual) {
		return nil
	}

	switch reflect.TypeOf(expected).Kind() {
	case reflect.Slice:
		var strategy ArrayComparisonStrategy
		if strategyFactory != nil {
			strategy = strategyFactory(currentPath)
		} else {
			strategy = StrategyExact(currentPath)
		}

		expectedVal := reflect.ValueOf(expected)
		actualVal := reflect.ValueOf(actual)

		expectedData := make([]interface{}, expectedVal.Len())
		actualData := make([]interface{}, actualVal.Len())

		for i := 0; i < expectedVal.Len(); i++ {
			expectedData[i] = expectedVal.Index(i).Interface()
		}
		for i := 0; i < actualVal.Len(); i++ {
			actualData[i] = actualVal.Index(i).Interface()
		}

		return strategy(expectedData, actualData)
	case reflect.Map:
		iter := reflect.ValueOf(expected).MapRange()

		for iter.Next() {
			actualValue := reflect.ValueOf(actual).MapIndex(iter.Key())

			if !actualValue.IsValid() {
				return &SubsetError{
					path:    []string{iter.Key().String()},
					message: "key is missing from map",
				}
			}

			newPath := currentPath + "/" + iter.Key().String()
			if err := IsSubset(iter.Value().Interface(), actualValue.Interface(), newPath, strategyFactory); err != nil {
				subsetErr, ok := err.(*SubsetError)
				if ok {
					subsetErr.AppendPath(iter.Key().String())
					return subsetErr
				}
				return err
			}
		}
	default:
		return &SubsetError{
			message: fmt.Sprintf("value mismatch, expected: %v != actual: %v", expected, actual),
		}
	}

	return nil
}

func StrategyAnywhere(path string) ArrayComparisonStrategy {
	return func(expectedData, actualData []interface{}) error {
		for i, expectedItem := range expectedData {
			matched := false
			for _, actualItem := range actualData {
				newPath := path + fmt.Sprintf("[%d]", i)
				if err := IsSubset(expectedItem, actualItem, newPath, StrategyFactory); err == nil {
					matched = true
					break
				}
			}
			if !matched {
				return &SubsetError{message: fmt.Sprintf("expected item %v not found in actual slice at path %s", expectedItem, path)}
			}
		}
		return nil
	}
}

func StrategyExact(path string) ArrayComparisonStrategy {
	return func(expectedData, actualData []interface{}) error {
		if len(expectedData) != len(actualData) {
			return &SubsetError{message: fmt.Sprintf("slice length mismatch at path %s: %d != %d", path, len(expectedData), len(actualData))}
		}
		for i, v := range expectedData {
			newPath := path + fmt.Sprintf("[%d]", i)
			if err := IsSubset(v, actualData[i], newPath, StrategyFactory); err != nil {
				return err
			}
		}
		return nil
	}
}
