package test

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	wildcard "github.com/IGLOU-EU/go-wildcard"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/discovery"
	"sigs.k8s.io/controller-runtime/pkg/client"

	harness "github.com/kyverno/kuttl/pkg/apis/testharness/v1beta1"
	"github.com/kyverno/kuttl/pkg/env"
	kfile "github.com/kyverno/kuttl/pkg/file"
	"github.com/kyverno/kuttl/pkg/http"
	testutils "github.com/kyverno/kuttl/pkg/test/utils"
)

// fileNameRegex contains two capturing groups to determine whether a file has special
// meaning (ex. assert) or contains an appliable object, and extra name elements.
var fileNameRegex = regexp.MustCompile(`^(?:\d+-)?([^-\.]+)(-[^\.]+)?(?:\.yaml)?$`)

type apply struct {
	object     client.Object
	shouldFail bool
}

type asserts struct {
	object  client.Object
	options *harness.Options
}

// A Step contains the name of the test step, its index in the test,
// and all of the test step's settings (including objects to apply and assert on).
type Step struct {
	Name       string
	Index      int
	SkipDelete bool

	Dir string

	Step   *harness.TestStep
	Assert *harness.TestAssert

	Asserts []asserts
	Apply   []apply
	Errors  []client.Object

	Timeout int

	Kubeconfig      string
	Client          func(forceNew bool) (client.Client, error)
	DiscoveryClient func() (discovery.DiscoveryInterface, error)

	Logger testutils.Logger
}

// Clean deletes all resources defined in the Apply list.
func (s *Step) Clean(namespace string) error {
	cl, err := s.Client(false)
	if err != nil {
		return err
	}

	dClient, err := s.DiscoveryClient()
	if err != nil {
		return err
	}

	for _, apply := range s.Apply {
		_, _, err := testutils.Namespaced(dClient, apply.object, namespace)
		if err != nil {
			return err
		}

		if err := cl.Delete(context.TODO(), apply.object); err != nil && !k8serrors.IsNotFound(err) {
			return err
		}
	}

	return nil
}

// DeleteExisting deletes any resources in the TestStep.Delete list prior to running the tests.
func (s *Step) DeleteExisting(namespace string) error {
	cl, err := s.Client(false)
	if err != nil {
		return err
	}

	dClient, err := s.DiscoveryClient()
	if err != nil {
		return err
	}

	toDelete := []client.Object{}

	if s.Step == nil {
		return nil
	}

	for _, ref := range s.Step.Delete {
		gvk := ref.GroupVersionKind()

		obj := testutils.NewResource(gvk.GroupVersion().String(), gvk.Kind, ref.Name, "")

		objNs := namespace
		if ref.Namespace != "" {
			objNs = ref.Namespace
		}

		_, objNs, err := testutils.Namespaced(dClient, obj, objNs)
		if err != nil {
			return err
		}

		if ref.Name == "" {
			u := &unstructured.UnstructuredList{}
			u.SetGroupVersionKind(gvk)

			listOptions := []client.ListOption{}

			if ref.Labels != nil {
				listOptions = append(listOptions, client.MatchingLabels(ref.Labels))
			}

			if objNs != "" {
				listOptions = append(listOptions, client.InNamespace(objNs))
			}

			err := cl.List(context.TODO(), u, listOptions...)
			if err != nil {
				return fmt.Errorf("listing matching resources: %w", err)
			}

			for index := range u.Items {
				toDelete = append(toDelete, &u.Items[index])
			}
		} else {
			// Otherwise just append the object specified.
			toDelete = append(toDelete, obj.DeepCopy())
		}
	}

	for _, obj := range toDelete {
		delete := &unstructured.Unstructured{}
		delete.SetGroupVersionKind(obj.GetObjectKind().GroupVersionKind())
		delete.SetName(obj.GetName())
		delete.SetNamespace(obj.GetNamespace())

		err := cl.Delete(context.TODO(), delete)
		if err != nil && !k8serrors.IsNotFound(err) {
			return err
		}
	}

	// Wait for resources to be deleted.
	return wait.PollImmediate(100*time.Millisecond, time.Duration(s.GetTimeout())*time.Second, func() (done bool, err error) {
		for _, obj := range toDelete {
			actual := &unstructured.Unstructured{}
			actual.SetGroupVersionKind(obj.GetObjectKind().GroupVersionKind())
			err = cl.Get(context.TODO(), testutils.ObjectKey(obj), actual)
			if err == nil || !k8serrors.IsNotFound(err) {
				return false, err
			}
		}

		return true, nil
	})
}

func doApply(test *testing.T, skipDelete bool, logger testutils.Logger, timeout int, dClient discovery.DiscoveryInterface, cl client.Client, obj client.Object, namespace string) error {
	_, _, err := testutils.Namespaced(dClient, obj, namespace)
	if err != nil {
		return err
	}
	ctx := context.Background()
	if timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, time.Duration(timeout)*time.Second)
		defer cancel()
	}
	updated, err := testutils.CreateOrUpdate(ctx, cl, obj, true)
	if err != nil {
		return err
	}
	// if the object was created, register cleanup
	if !updated && !skipDelete {
		test.Cleanup(func() {
			ctx := context.Background()
			if timeout > 0 {
				var cancel context.CancelFunc
				ctx, cancel = context.WithTimeout(ctx, time.Duration(timeout)*time.Second)
				defer cancel()
			}
			if err := wait.PollImmediateUntilWithContext(ctx, 100*time.Millisecond, func(ctx context.Context) (bool, error) {
				if err := cl.Delete(ctx, obj); err == nil || k8serrors.IsNotFound(err) {
					return true, nil
				}
				return false, nil
			}); err != nil && !k8serrors.IsNotFound(err) {
				test.Error(err)
			} else {
				if err := wait.PollImmediateUntilWithContext(ctx, 100*time.Millisecond, func(ctx context.Context) (bool, error) {
					err := cl.Get(ctx, testutils.ObjectKey(obj), obj)
					if k8serrors.IsNotFound(err) {
						return true, nil
					}
					return false, nil
				}); err != nil {
					test.Error(err)
				} else {
					logger.Log(testutils.ResourceID(obj), "deleted")
				}
			}
		})
	}
	action := "created"
	if updated {
		action = "updated"
	}
	logger.Log(testutils.ResourceID(obj), action)
	return nil
}

// Create applies all resources defined in the Apply list.
func (s *Step) Create(test *testing.T, namespace string) []error {
	cl, err := s.Client(true)
	if err != nil {
		return []error{err}
	}

	dClient, err := s.DiscoveryClient()
	if err != nil {
		return []error{err}
	}

	errs := []error{}

	for _, apply := range s.Apply {
		err := doApply(test, s.SkipDelete, s.Logger, s.Timeout, dClient, cl, apply.object, namespace)
		if err != nil && !apply.shouldFail {
			errs = append(errs, err)
		}
		// if there was no error but we expected one
		if err == nil && apply.shouldFail {
			// TODO: improve error message
			errs = append(errs, errors.New("an error was expected but didn't happen"))
		}
	}

	return errs
}

// GetTimeout gets the timeout defined for the test step.
func (s *Step) GetTimeout() int {
	timeout := s.Timeout
	if s.Assert != nil && s.Assert.Timeout != 0 {
		timeout = s.Assert.Timeout
	}
	return timeout
}

func list(cl client.Client, gvk schema.GroupVersionKind, namespace string) ([]unstructured.Unstructured, error) {
	list := unstructured.UnstructuredList{}
	list.SetGroupVersionKind(gvk)

	listOptions := []client.ListOption{}
	if namespace != "" {
		listOptions = append(listOptions, client.InNamespace(namespace))
	}

	if err := cl.List(context.TODO(), &list, listOptions...); err != nil {
		return []unstructured.Unstructured{}, err
	}

	return list.Items, nil
}

// CheckResource checks if the expected resource's state in Kubernetes is correct.
func (s *Step) CheckResource(expected runtime.Object, namespace string, strategyFactory testutils.ArrayComparisonStrategyFactory) []error {
	cl, err := s.Client(false)
	if err != nil {
		return []error{err}
	}

	dClient, err := s.DiscoveryClient()
	if err != nil {
		return []error{err}
	}

	testErrors := []error{}

	name, namespace, err := testutils.Namespaced(dClient, expected, namespace)
	if err != nil {
		return append(testErrors, err)
	}

	gvk := expected.GetObjectKind().GroupVersionKind()

	actuals := []unstructured.Unstructured{}

	if name != "" {
		actual := unstructured.Unstructured{}
		actual.SetGroupVersionKind(gvk)

		err = cl.Get(context.TODO(), client.ObjectKey{
			Namespace: namespace,
			Name:      name,
		}, &actual)

		actuals = append(actuals, actual)
	} else {
		actuals, err = list(cl, gvk, namespace)
		if len(actuals) == 0 {
			testErrors = append(testErrors, fmt.Errorf("no resources matched of kind: %s", gvk.String()))
		}
	}
	if err != nil {
		return append(testErrors, err)
	}

	expectedObj, err := runtime.DefaultUnstructuredConverter.ToUnstructured(expected)
	if err != nil {
		return append(testErrors, err)
	}

	for _, actual := range actuals {
		actual := actual

		tmpTestErrors := []error{}

		if err := testutils.IsSubset(expectedObj, actual.UnstructuredContent(), "/", strategyFactory); err != nil {
			diff, diffErr := testutils.PrettyDiff(expected, &actual)
			if diffErr == nil {
				tmpTestErrors = append(tmpTestErrors, fmt.Errorf(diff))
			} else {
				tmpTestErrors = append(tmpTestErrors, diffErr)
			}

			tmpTestErrors = append(tmpTestErrors, fmt.Errorf("resource %s: %s", testutils.ResourceID(expected), err))
		}

		if len(tmpTestErrors) == 0 {
			return tmpTestErrors
		}

		testErrors = append(testErrors, tmpTestErrors...)
	}

	return testErrors
}

// CheckResourceAbsent checks if the expected resource's state is absent in Kubernetes.
func (s *Step) CheckResourceAbsent(expected runtime.Object, namespace string) error {
	cl, err := s.Client(false)
	if err != nil {
		return err
	}

	dClient, err := s.DiscoveryClient()
	if err != nil {
		return err
	}

	name, namespace, err := testutils.Namespaced(dClient, expected, namespace)
	if err != nil {
		return err
	}

	gvk := expected.GetObjectKind().GroupVersionKind()

	var actuals []unstructured.Unstructured

	if name != "" {
		actual := unstructured.Unstructured{}
		actual.SetGroupVersionKind(gvk)

		if err := cl.Get(context.TODO(), client.ObjectKey{
			Namespace: namespace,
			Name:      name,
		}, &actual); err != nil {
			if k8serrors.IsNotFound(err) {
				return nil
			}

			return err
		}

		actuals = []unstructured.Unstructured{actual}
	} else {
		actuals, err = list(cl, gvk, namespace)
		if err != nil {
			return err
		}
	}

	expectedObj, err := runtime.DefaultUnstructuredConverter.ToUnstructured(expected)
	if err != nil {
		return err
	}

	var unexpectedObjects []unstructured.Unstructured
	for _, actual := range actuals {
		if err := testutils.IsSubset(expectedObj, actual.UnstructuredContent(), "/", nil); err == nil {
			unexpectedObjects = append(unexpectedObjects, actual)
		}
	}

	if len(unexpectedObjects) == 0 {
		return nil
	}
	if len(unexpectedObjects) == 1 {
		return fmt.Errorf("resource %s %s matched error assertion", unexpectedObjects[0].GroupVersionKind(), unexpectedObjects[0].GetName())
	}
	return fmt.Errorf("resource %s %s (and %d other resources) matched error assertion", unexpectedObjects[0].GroupVersionKind(), unexpectedObjects[0].GetName(), len(unexpectedObjects)-1)
}

// pathMatches checks if the given path matches the pattern.
func pathMatches(pattern, path string) bool {
	return wildcard.Match(strings.TrimSuffix(pattern, "/"), path)
}

func metaTypeMatches(assertArray harness.AssertArray, obj client.Object) bool {
	if assertArray.Match != nil {
		expected, err := runtime.DefaultUnstructuredConverter.ToUnstructured(assertArray.Match)
		if err != nil {
			return false
		}
		actual, err := runtime.DefaultUnstructuredConverter.ToUnstructured(obj)
		if err != nil {
			return false
		}
		if err := testutils.IsSubset(expected, actual, "/", testutils.DefaultStrategyFactory()); err != nil {
			return false
		}
	}
	return true
}

// Build StrategyFactory for IsSubset
func NewStrategyFactory(a asserts) func(path string) testutils.ArrayComparisonStrategy {
	var strategyFactory func(path string) testutils.ArrayComparisonStrategy
	recursiveStrategyFactory := func(path string) testutils.ArrayComparisonStrategy {
		if a.options != nil && len(a.options.AssertArray) > 0 {
			for _, assertArr := range a.options.AssertArray {
				if pathMatches(assertArr.Path, path) && metaTypeMatches(assertArr, a.object) {
					switch assertArr.Strategy {
					case harness.StrategyExact:
						return testutils.StrategyExact(path, strategyFactory)
					case harness.StrategyAnywhere:
						return testutils.StrategyAnywhere(path, strategyFactory)
					}
				}
			}
		}
		// Default strategy if no match is found
		return testutils.StrategyExact(path, strategyFactory)
	}
	strategyFactory = recursiveStrategyFactory
	return strategyFactory
}

// CheckAssertCommands Runs the commands provided in `commands` and check if have been run successfully.
// the errors returned can be a a failure of executing the command or the failure of the command executed.
func (s *Step) CheckAssertCommands(ctx context.Context, namespace string, commands []harness.TestAssertCommand, timeout int) []error {
	testErrors := []error{}
	if _, err := testutils.RunAssertCommands(ctx, s.Logger, namespace, commands, "", timeout, s.Kubeconfig); err != nil {
		testErrors = append(testErrors, err)
	}
	return testErrors
}

// Check checks if the resources defined in Asserts and Errors are in the correct state.
func (s *Step) Check(namespace string, timeout int) []error {
	testErrors := []error{}

	for _, expected := range s.Asserts {
		strategyFactory := NewStrategyFactory(expected)
		testErrors = append(testErrors, s.CheckResource(expected.object, namespace, strategyFactory)...)
	}

	if s.Assert != nil {
		testErrors = append(testErrors, s.CheckAssertCommands(context.TODO(), namespace, s.Assert.Commands, timeout)...)
	}

	for _, expected := range s.Errors {
		if testError := s.CheckResourceAbsent(expected, namespace); testError != nil {
			testErrors = append(testErrors, testError)
		}
	}

	return testErrors
}

// Run runs a KUTTL test step:
// 1. Apply all desired objects to Kubernetes.
// 2. Wait for all of the states defined in the test step's asserts to be true.'
func (s *Step) Run(test *testing.T, namespace string) []error {
	s.Logger.Log("starting test step", s.String())

	if err := s.DeleteExisting(namespace); err != nil {
		return []error{err}
	}

	testErrors := []error{}

	if s.Step != nil {
		for _, command := range s.Step.Commands {
			if command.Background {
				s.Logger.Log("background commands are not allowed for steps and will be run in foreground")
				command.Background = false
			}
		}
		if _, err := testutils.RunCommands(context.TODO(), s.Logger, namespace, s.Step.Commands, s.Dir, s.Timeout, s.Kubeconfig); err != nil {
			testErrors = append(testErrors, err)
		}
	}

	testErrors = append(testErrors, s.Create(test, namespace)...)

	if len(testErrors) != 0 {
		return testErrors
	}

	timeoutF := float64(s.GetTimeout())
	start := time.Now()

	for elapsed := 0.0; elapsed < timeoutF; elapsed = time.Since(start).Seconds() {
		testErrors = s.Check(namespace, int(timeoutF-elapsed))

		if len(testErrors) == 0 {
			break
		}
		if hasTimeoutErr(testErrors) {
			break
		}
		time.Sleep(time.Second)
	}

	// all is good
	if len(testErrors) == 0 {
		s.Logger.Log("test step completed", s.String())
		return testErrors
	}
	// test failure processing
	s.Logger.Log("test step failed", s.String())
	if s.Assert == nil {
		return testErrors
	}
	for _, collector := range s.Assert.Collectors {
		s.Logger.Logf("collecting log output for %s", collector.String())
		if collector.Command() == nil {
			s.Logger.Log("skipping invalid assertion collector")
			continue
		}
		_, err := testutils.RunCommand(context.TODO(), namespace, *collector.Command(), s.Dir, s.Logger, s.Logger, s.Logger, s.Timeout, s.Kubeconfig)
		if err != nil {
			s.Logger.Log("post assert collector failure: %s", err)
		}
	}
	s.Logger.Flush()
	return testErrors
}

// String implements the string interface, returning the name of the test step.
func (s *Step) String() string {
	return fmt.Sprintf("%d-%s", s.Index, s.Name)
}

// LoadYAML loads the resources from a YAML file for a test step:
//   - If the YAML file is called "assert", then it contains objects to
//     add to the test step's list of assertions.
//   - If the YAML file is called "errors", then it contains objects that,
//     if seen, mark a test immediately failed.
//   - All other YAML files are considered resources to create.
func (s *Step) LoadYAML(file string) error {
	objects, err := testutils.LoadYAMLFromFile(file)
	if err != nil {
		return fmt.Errorf("loading %s: %s", file, err)
	}

	if err = s.populateObjectsByFileName(filepath.Base(file), objects); err != nil {
		return fmt.Errorf("populating step: %v", err)
	}

	asserties := []asserts{}

	for _, assert := range s.Asserts {
		obj := assert.object
		if obj.GetObjectKind().GroupVersionKind().Kind == "TestAssert" {
			if testAssert, ok := obj.DeepCopyObject().(*harness.TestAssert); ok {
				s.Assert = testAssert
			} else {
				return fmt.Errorf("failed to load TestAssert object from %s: it contains an object of type %T", file, obj)
			}
		} else {
			asserties = append(asserties, assert)
		}
	}

	applies := []apply{}

	for _, apply := range s.Apply {
		if apply.object.GetObjectKind().GroupVersionKind().Kind == "TestStep" {
			if testStep, ok := apply.object.(*harness.TestStep); ok {
				// Validate TestStep
				if err := validateTestStep(testStep, s.Dir); err != nil {
					return fmt.Errorf("failed to validate TestStep object from %s: %v", file, err)
				}
				if s.Step != nil {
					return fmt.Errorf("more than 1 TestStep not allowed in step %q", s.Name)
				}
				s.Step = testStep
			} else {
				return fmt.Errorf("failed to load TestStep object from %s: it contains an object of type %T", file, apply.object)
			}
			s.Step.Index = s.Index
			if s.Step.Name != "" {
				s.Name = s.Step.Name
			}
			if s.Step.Kubeconfig != "" {
				exKubeconfig := env.Expand(s.Step.Kubeconfig)
				s.Kubeconfig = cleanPath(exKubeconfig, s.Dir)
			}
		} else {
			applies = append(applies, apply)
		}
	}

	// process provided steps configured TestStep kind
	if s.Step != nil {
		// process configured step applies
		for _, applyPath := range s.Step.Apply {
			exApply := env.Expand(applyPath.File)
			aa, err := ObjectsFromPath(exApply, s.Dir)
			if err != nil {
				return fmt.Errorf("step %q apply path %s: %w", s.Name, exApply, err)
			}
			for _, a := range aa {
				applies = append(applies, apply{object: a, shouldFail: applyPath.ShouldFail})
			}
		}
		// process configured step asserts
		for _, assertPath := range s.Step.Assert {
			exAssert := env.Expand(assertPath.File)
			assert, err := ObjectsFromPath(exAssert, s.Dir)
			if err != nil {
				return fmt.Errorf("step %q assert path %s: %w", s.Name, exAssert, err)
			}
			for _, a := range assert {
				asserties = append(asserties, asserts{object: a})
			}
		}
		// process configured errors
		for _, errorPath := range s.Step.Error {
			exError := env.Expand(errorPath)
			errObjs, err := ObjectsFromPath(exError, s.Dir)
			if err != nil {
				return fmt.Errorf("step %q error path %s: %w", s.Name, exError, err)
			}
			s.Errors = append(s.Errors, errObjs...)
		}
	}

	s.Apply = applies
	s.Asserts = asserties
	return nil
}

// populateObjectsByFileName populates s.Asserts, s.Errors, and/or s.Apply for files containing
// "assert", "errors", or no special string, respectively.
func (s *Step) populateObjectsByFileName(fileName string, objects []client.Object) error {
	matches := fileNameRegex.FindStringSubmatch(fileName)
	if len(matches) < 2 {
		return fmt.Errorf("%s does not match file name regexp: %s", fileName, testStepRegex.String())
	}

	switch fname := strings.ToLower(matches[1]); fname {
	case "assert":
		for _, obj := range objects {
			s.Asserts = append(s.Asserts, asserts{object: obj})
		}
	case "errors":
		s.Errors = append(s.Errors, objects...)
	default:
		if s.Name == "" {
			if len(matches) > 2 {
				// The second matching group will already have a hyphen prefix.
				s.Name = matches[1] + matches[2]
			} else {
				s.Name = matches[1]
			}
		}
		for _, a := range objects {
			s.Apply = append(s.Apply, apply{object: a})
		}
	}

	return nil
}

// ObjectsFromPath returns an array of runtime.Objects for files / urls provided
func ObjectsFromPath(path, dir string) ([]client.Object, error) {
	if http.IsURL(path) {
		apply, err := http.ToObjects(path)
		if err != nil {
			return nil, err
		}
		return apply, nil
	}

	// it's a directory or file
	cPath := cleanPath(path, dir)
	paths, err := kfile.FromPath(cPath, "*.yaml")
	if err != nil {
		return nil, fmt.Errorf("failed to find YAML files in %s: %w", cPath, err)
	}
	apply, err := kfile.ToObjects(paths)
	if err != nil {
		return nil, err
	}
	return apply, nil
}

// cleanPath returns either the abs path or the joined path
func cleanPath(path, dir string) string {
	if filepath.IsAbs(path) {
		return path
	}
	return filepath.Join(dir, path)
}

func hasTimeoutErr(err []error) bool {
	for i := range err {
		if errors.Is(err[i], context.DeadlineExceeded) {
			return true
		}
	}
	return false
}

func validateTestStep(ts *harness.TestStep, baseDir string) error {
	// Check if referenced files in Apply exist
	for _, apply := range ts.Apply {
		path := filepath.Join(baseDir, apply.File)
		if _, err := os.Stat(path); os.IsNotExist(err) {
			return fmt.Errorf("referenced file in Apply does not exist: %s", path)
		}
	}
	// Check if referenced files in  Assert  exist
	for _, assert := range ts.Assert {
		path := filepath.Join(baseDir, assert.File)
		if _, err := os.Stat(path); os.IsNotExist(err) {
			return fmt.Errorf("referenced file in Assert does not exist: %s", path)
		}
	}
	// Check if referenced files in  Error exist
	for _, errorPath := range ts.Error {
		path := filepath.Join(baseDir, errorPath)
		if _, err := os.Stat(path); os.IsNotExist(err) {
			return fmt.Errorf("referenced file in Error does not exist: %s", path)
		}
	}

	return nil
}
