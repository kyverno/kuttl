package test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	harness "github.com/kyverno/kuttl/pkg/apis/testharness/v1beta1"
	testutils "github.com/kyverno/kuttl/pkg/test/utils"
)

const (
	testNamespace = "world"
)

// Verify the test state as loaded from disk.
// Each test provides a path to a set of test steps and their rendered result.
func TestStepClean(t *testing.T) {
	pod := testutils.NewPod("hello", "")

	podWithNamespace := testutils.WithNamespace(pod, testNamespace)
	pod2WithNamespace := testutils.NewPod("hello2", testNamespace)
	pod2WithDiffNamespace := testutils.NewPod("hello2", "different-namespace")

	cl := fake.NewClientBuilder().WithObjects(pod, pod2WithNamespace, pod2WithDiffNamespace).WithScheme(scheme.Scheme).Build()

	step := Step{
		Apply: []apply{
			{object: pod},
			{object: pod2WithDiffNamespace},
			{object: testutils.NewPod("does-not-exist", "")},
		},
		Client:          func(bool) (client.Client, error) { return cl, nil },
		DiscoveryClient: func() (discovery.DiscoveryInterface, error) { return testutils.FakeDiscoveryClient(), nil },
	}

	assert.Nil(t, step.Clean(testNamespace))

	assert.True(t, k8serrors.IsNotFound(cl.Get(context.TODO(), testutils.ObjectKey(podWithNamespace), podWithNamespace)))
	assert.Nil(t, cl.Get(context.TODO(), testutils.ObjectKey(pod2WithNamespace), pod2WithNamespace))
	assert.True(t, k8serrors.IsNotFound(cl.Get(context.TODO(), testutils.ObjectKey(pod2WithDiffNamespace), pod2WithDiffNamespace)))
}

// Verify the test state as loaded from disk.
// Each test provides a path to a set of test steps and their rendered result.
func TestStepCreate(t *testing.T) {
	pod := testutils.NewPod("hello", "default")
	podWithNamespace := testutils.NewPod("hello2", "different-namespace")
	clusterScopedResource := testutils.NewResource("v1", "Namespace", "my-namespace", "default")
	podToUpdate := testutils.NewPod("update-me", "default")
	specToApply := map[string]interface{}{
		"containers":    nil,
		"restartPolicy": "OnFailure",
	}

	updateToApply := testutils.WithSpec(t, podToUpdate, specToApply)

	cl := fake.NewClientBuilder().WithScheme(scheme.Scheme).WithRuntimeObjects(testutils.WithNamespace(podToUpdate, testNamespace)).Build()

	step := Step{
		Logger: testutils.NewTestLogger(t, ""),
		Apply: []apply{
			{object: pod.DeepCopy()},
			{object: podWithNamespace.DeepCopy()},
			{object: clusterScopedResource},
			{object: updateToApply},
		},
		Client:          func(bool) (client.Client, error) { return cl, nil },
		DiscoveryClient: func() (discovery.DiscoveryInterface, error) { return testutils.FakeDiscoveryClient(), nil },
	}

	assert.Equal(t, []error{}, step.Create(t, testNamespace))

	assert.Nil(t, cl.Get(context.TODO(), testutils.ObjectKey(pod), pod))
	assert.Nil(t, cl.Get(context.TODO(), testutils.ObjectKey(clusterScopedResource), clusterScopedResource))

	updatedPod := &unstructured.Unstructured{Object: map[string]interface{}{"apiVersion": "v1", "kind": "Pod"}}
	assert.Nil(t, cl.Get(context.TODO(), testutils.ObjectKey(podToUpdate), updatedPod))
	assert.Equal(t, specToApply, updatedPod.Object["spec"])

	assert.Nil(t, cl.Get(context.TODO(), testutils.ObjectKey(podWithNamespace), podWithNamespace))
	actual := testutils.NewPod("hello2", testNamespace)
	assert.True(t, k8serrors.IsNotFound(cl.Get(context.TODO(), testutils.ObjectKey(actual), actual)))
}

// Verify that the DeleteExisting method properly cleans up resources during a test step.
func TestStepDeleteExisting(t *testing.T) {
	podToDelete := testutils.NewPod("delete-me", testNamespace)
	podToDeleteDefaultNS := testutils.NewPod("also-delete-me", "default")
	podToKeep := testutils.NewPod("keep-me", testNamespace)

	cl := fake.NewClientBuilder().WithScheme(scheme.Scheme).WithRuntimeObjects(podToDelete, podToKeep, podToDeleteDefaultNS).Build()

	step := Step{
		Logger: testutils.NewTestLogger(t, ""),
		Step: &harness.TestStep{
			Delete: []harness.ObjectReference{
				{
					ObjectReference: corev1.ObjectReference{
						Kind:       "Pod",
						APIVersion: "v1",
						Name:       "delete-me",
					},
				},
				{
					ObjectReference: corev1.ObjectReference{
						Kind:       "Pod",
						APIVersion: "v1",
						Name:       "also-delete-me",
						Namespace:  "default",
					},
				},
			},
		},
		Client:          func(bool) (client.Client, error) { return cl, nil },
		DiscoveryClient: func() (discovery.DiscoveryInterface, error) { return testutils.FakeDiscoveryClient(), nil },
	}

	assert.Nil(t, cl.Get(context.TODO(), testutils.ObjectKey(podToKeep), podToKeep))
	assert.Nil(t, cl.Get(context.TODO(), testutils.ObjectKey(podToDelete), podToDelete))
	assert.Nil(t, cl.Get(context.TODO(), testutils.ObjectKey(podToDeleteDefaultNS), podToDeleteDefaultNS))

	assert.Nil(t, step.DeleteExisting(testNamespace))

	assert.Nil(t, cl.Get(context.TODO(), testutils.ObjectKey(podToKeep), podToKeep))
	assert.True(t, k8serrors.IsNotFound(cl.Get(context.TODO(), testutils.ObjectKey(podToDelete), podToDelete)))
	assert.True(t, k8serrors.IsNotFound(cl.Get(context.TODO(), testutils.ObjectKey(podToDeleteDefaultNS), podToDeleteDefaultNS)))
}

func TestCheckResource(t *testing.T) {
	for _, test := range []struct {
		testName    string
		actual      runtime.Object
		expected    runtime.Object
		shouldError bool
	}{
		{
			testName: "resource matches",
			actual:   testutils.NewPod("hello", ""),
			expected: testutils.NewPod("hello", ""),
		},
		{
			testName:    "resource mis-match",
			actual:      testutils.NewPod("hello", ""),
			expected:    testutils.WithSpec(t, testutils.NewPod("hello", ""), map[string]interface{}{"invalid": "key"}),
			shouldError: true,
		},
		{
			testName: "resource subset match",
			actual: testutils.WithSpec(t, testutils.NewPod("hello", ""), map[string]interface{}{
				"containers":    nil,
				"restartPolicy": "OnFailure",
			}),
			expected: testutils.WithSpec(t, testutils.NewPod("hello", ""), map[string]interface{}{
				"restartPolicy": "OnFailure",
			}),
		},
		{
			testName:    "resource does not exist",
			actual:      testutils.NewPod("other", ""),
			expected:    testutils.NewPod("hello", ""),
			shouldError: true,
		},
	} {
		test := test

		t.Run(test.testName, func(t *testing.T) {
			fakeDiscovery := testutils.FakeDiscoveryClient()
			namespace := testNamespace

			_, _, err := testutils.Namespaced(fakeDiscovery, test.actual, namespace)
			assert.Nil(t, err)

			step := Step{
				Logger: testutils.NewTestLogger(t, ""),
				Client: func(bool) (client.Client, error) {
					return fake.NewClientBuilder().WithScheme(scheme.Scheme).WithRuntimeObjects(test.actual).Build(), nil
				},
				DiscoveryClient: func() (discovery.DiscoveryInterface, error) { return fakeDiscovery, nil },
			}

			errors := step.CheckResource(test.expected, namespace, nil)

			if test.shouldError {
				assert.NotEqual(t, []error{}, errors)
			} else {
				assert.Equal(t, []error{}, errors)
			}
		})
	}
}

func TestCheckResourceAbsent(t *testing.T) {
	for _, test := range []struct {
		name        string
		actual      []runtime.Object
		expected    runtime.Object
		shouldError bool
		expectedErr string
	}{
		{
			name:        "resource matches",
			actual:      []runtime.Object{testutils.NewPod("hello", "")},
			expected:    testutils.NewPod("hello", ""),
			shouldError: true,
		},
		{
			name: "one of more resources matches",
			actual: []runtime.Object{
				testutils.NewV1Pod("pod1", "", "val1"),
				testutils.NewV1Pod("pod2", "", "val2"),
			},
			expected:    testutils.WithSpec(t, testutils.NewPod("", ""), map[string]interface{}{"serviceAccountName": "val1"}),
			shouldError: true,
			expectedErr: "resource /v1, Kind=Pod pod1 matched error assertion",
		},
		{
			name: "multiple of more resources matches",
			actual: []runtime.Object{
				testutils.NewV1Pod("pod1", "", "val1"),
				testutils.NewV1Pod("pod2", "", "val1"),
				testutils.NewV1Pod("pod3", "", "val2"),
			},
			expected:    testutils.WithSpec(t, testutils.NewPod("", ""), map[string]interface{}{"serviceAccountName": "val1"}),
			shouldError: true,
			expectedErr: "resource /v1, Kind=Pod pod1 (and 1 other resources) matched error assertion",
		},
		{
			name:     "resource mis-match",
			actual:   []runtime.Object{testutils.NewPod("hello", "")},
			expected: testutils.WithSpec(t, testutils.NewPod("hello", ""), map[string]interface{}{"invalid": "key"}),
		},
		{
			name:     "resource does not exist",
			actual:   []runtime.Object{testutils.NewPod("other", "")},
			expected: testutils.NewPod("hello", ""),
		},
	} {
		test := test

		t.Run(test.name, func(t *testing.T) {
			fakeDiscovery := testutils.FakeDiscoveryClient()

			for _, object := range test.actual {
				_, _, err := testutils.Namespaced(fakeDiscovery, object, testNamespace)
				assert.NoError(t, err)
			}

			step := Step{
				Logger: testutils.NewTestLogger(t, ""),
				Client: func(bool) (client.Client, error) {
					return fake.NewClientBuilder().WithScheme(scheme.Scheme).WithRuntimeObjects(test.actual...).Build(), nil
				},
				DiscoveryClient: func() (discovery.DiscoveryInterface, error) { return fakeDiscovery, nil },
			}

			error := step.CheckResourceAbsent(test.expected, testNamespace)

			if test.shouldError {
				assert.Error(t, error)
				if test.expectedErr != "" {
					assert.EqualError(t, error, test.expectedErr)
				}
			} else {
				assert.NoError(t, error)
			}
		})
	}
}

func TestRun(t *testing.T) {
	for _, test := range []struct {
		testName     string
		shouldError  bool
		Step         Step
		updateMethod func(*testing.T, client.Client)
	}{
		{
			testName: "successful run",
			Step: Step{
				Apply: []apply{
					{object: testutils.NewPod("hello", "")},
				},
				Asserts: []asserts{
					{object: testutils.NewPod("hello", "")},
				},
			},
		},
		{
			"failed run", true, Step{
				Apply: []apply{
					{object: testutils.NewPod("hello", "")},
				},
				Asserts: []asserts{
					{object: testutils.WithStatus(t, testutils.NewPod("hello", ""), map[string]interface{}{
						"phase": "Ready",
					})},
				},
			}, nil,
		},
		{
			"delayed run", false, Step{
				Apply: []apply{
					{object: testutils.NewPod("hello", "")},
				},
				Asserts: []asserts{
					{object: testutils.WithStatus(t, testutils.NewPod("hello", ""), map[string]interface{}{
						"phase": "Ready",
					})},
				},
			}, func(t *testing.T, client client.Client) {
				pod := testutils.NewPod("hello", testNamespace)
				assert.Nil(t, client.Get(context.TODO(), types.NamespacedName{Namespace: testNamespace, Name: "hello"}, pod))

				// mock kubelet to set the pod status
				assert.Nil(t, client.Update(context.TODO(), testutils.WithStatus(t, pod, map[string]interface{}{
					"phase": "Ready",
				})))
			},
		},
	} {
		test := test

		t.Run(test.testName, func(t *testing.T) {
			test.Step.Assert = &harness.TestAssert{
				Timeout: 1,
			}

			cl := fake.NewClientBuilder().WithScheme(scheme.Scheme).Build()

			test.Step.Client = func(bool) (client.Client, error) { return cl, nil }
			test.Step.DiscoveryClient = func() (discovery.DiscoveryInterface, error) { return testutils.FakeDiscoveryClient(), nil }
			test.Step.Logger = testutils.NewTestLogger(t, "")

			if test.updateMethod != nil {
				test.Step.Assert.Timeout = 10

				go func() {
					time.Sleep(time.Second * 2)
					test.updateMethod(t, cl)
				}()
			}

			errors := test.Step.Run(t, testNamespace)

			if test.shouldError {
				assert.NotEqual(t, []error{}, errors)
			} else {
				assert.Equal(t, []error{}, errors)
			}
		})
	}
}

func TestPopulateObjectsByFileName(t *testing.T) {
	for _, tt := range []struct {
		fileName                   string
		isAssert, isError, isApply bool
		name                       string
		errExp                     bool
	}{
		{"00-assert.yaml", true, false, false, "", false},
		{"00-errors.yaml", false, true, false, "", false},
		{"00-foo.yaml", false, false, true, "foo", false},
		{"123-assert.yaml", true, false, false, "", false},
		{"123-errors.yaml", false, true, false, "", false},
		{"123-foo.yaml", false, false, true, "foo", false},
		{"00-assert-bar.yaml", true, false, false, "", false},
		{"00-errors-bar.yaml", false, true, false, "", false},
		{"00-foo-bar.yaml", false, false, true, "foo-bar", false},
		{"00-foo-bar-baz.yaml", false, false, true, "foo-bar-baz", false},
	} {
		tt := tt

		t.Run(tt.fileName, func(t *testing.T) {
			step := &Step{}
			err := step.populateObjectsByFileName(tt.fileName, []client.Object{testutils.NewPod("foo", "")})
			assert.Nil(t, err)
			assert.Equal(t, tt.isAssert, len(step.Asserts) != 0)
			assert.Equal(t, tt.isError, len(step.Errors) != 0)
			assert.Equal(t, tt.isApply, len(step.Apply) != 0)
			if tt.isApply && len(step.Apply) != 0 {
				assert.Equal(t, tt.name, step.Name)
			}
		})
	}
}

func TestPathMatches(t *testing.T) {
	tests := []struct {
		name     string
		pattern  string
		path     string
		expected bool
	}{
		{
			name:     "Exact match",
			pattern:  "/api/v1/users",
			path:     "/api/v1/users",
			expected: true,
		},
		{
			name:     "Wildcard at end matches",
			pattern:  "/api/v1/*",
			path:     "/api/v1/users",
			expected: true,
		},
		{
			name:     "Multiple wildcards match",
			pattern:  "/api/*/users/*",
			path:     "/api/v1/users/1234",
			expected: true,
		},
		{
			name:     "Wildcard in middle doesn't match",
			pattern:  "/api/*/users",
			path:     "/api/v1/admins",
			expected: false,
		},
		{
			name:     "Mismatch at the end",
			pattern:  "/api/v1/users",
			path:     "/api/v1/users/extra",
			expected: false,
		},
		{
			name:     "Pattern with trailing slash",
			pattern:  "/api/v1/users/",
			path:     "/api/v1/users",
			expected: true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			result := pathMatches(test.pattern, test.path)
			assert.Equal(t, test.expected, result)
		})
	}
}

func TestMetaTypeMatches(t *testing.T) {
	tests := []struct {
		name         string
		assertArray  harness.AssertArray
		obj          *metav1.PartialObjectMetadata
		expectedBool bool
	}{
		{
			name: "Matching Subset",
			assertArray: harness.AssertArray{
				Match: &metav1.PartialObjectMetadata{
					ObjectMeta: metav1.ObjectMeta{
						Annotations: map[string]string{
							"key": "value",
						},
					},
				},
			},
			obj: &metav1.PartialObjectMetadata{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						"key": "value",
						"foo": "bar",
					},
				},
			},
			expectedBool: true,
		},
		{
			name: "Non-matching Subset",
			assertArray: harness.AssertArray{
				Match: &metav1.PartialObjectMetadata{
					ObjectMeta: metav1.ObjectMeta{
						Annotations: map[string]string{
							"key": "wrongValue",
						},
					},
				},
			},
			obj: &metav1.PartialObjectMetadata{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						"key": "value",
						"foo": "bar",
					},
				},
			},
			expectedBool: false,
		},
		{
			name: "Empty Annotations in Match",
			assertArray: harness.AssertArray{
				Match: &metav1.PartialObjectMetadata{
					ObjectMeta: metav1.ObjectMeta{
						Annotations: map[string]string{},
					},
				},
			},
			obj: &metav1.PartialObjectMetadata{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						"key": "value",
						"foo": "bar",
					},
				},
			},
			expectedBool: true,
		},
		{
			name: "Using Labels for Matching",
			assertArray: harness.AssertArray{
				Match: &metav1.PartialObjectMetadata{
					ObjectMeta: metav1.ObjectMeta{
						Labels: map[string]string{
							"app": "test",
						},
					},
				},
			},
			obj: &metav1.PartialObjectMetadata{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						"app": "test",
						"env": "prod",
					},
				},
			},
			expectedBool: true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			result := metaTypeMatches(test.assertArray, test.obj)
			assert.Equal(t, test.expectedBool, result)
		})
	}
}
