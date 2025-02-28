/*
Copyright 2018 The Knative Authors

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
package v1alpha1

import (
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"knative.dev/pkg/apis"
	"knative.dev/pkg/apis/duck"
	duckv1beta1 "knative.dev/pkg/apis/duck/v1beta1"
	apitest "knative.dev/pkg/apis/testing"
	net "knative.dev/serving/pkg/apis/networking"
	"knative.dev/serving/pkg/apis/serving"
)

func TestRevisionDuckTypes(t *testing.T) {
	tests := []struct {
		name string
		t    duck.Implementable
	}{{
		name: "conditions",
		t:    &duckv1beta1.Conditions{},
	}}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := duck.VerifyType(&Revision{}, test.t)
			if err != nil {
				t.Errorf("VerifyType(Revision, %T) = %v", test.t, err)
			}
		})
	}
}

func TestIsActivationRequired(t *testing.T) {
	cases := []struct {
		name                 string
		status               RevisionStatus
		isActivationRequired bool
	}{{
		name:                 "empty status should not be inactive",
		status:               RevisionStatus{},
		isActivationRequired: false,
	}, {
		name: "Ready status should not be inactive",
		status: RevisionStatus{
			Status: duckv1beta1.Status{
				Conditions: duckv1beta1.Conditions{{
					Type:   RevisionConditionReady,
					Status: corev1.ConditionTrue,
				}},
			},
		},
		isActivationRequired: false,
	}, {
		name: "Inactive status should be inactive",
		status: RevisionStatus{
			Status: duckv1beta1.Status{
				Conditions: duckv1beta1.Conditions{{
					Type:   RevisionConditionActive,
					Status: corev1.ConditionFalse,
				}},
			},
		},
		isActivationRequired: true,
	}, {
		name: "Updating status should be inactive",
		status: RevisionStatus{
			Status: duckv1beta1.Status{
				Conditions: duckv1beta1.Conditions{{
					Type:   RevisionConditionReady,
					Status: corev1.ConditionUnknown,
					Reason: "Updating",
				}, {
					Type:   RevisionConditionActive,
					Status: corev1.ConditionUnknown,
					Reason: "Updating",
				}},
			},
		},
		isActivationRequired: true,
	}, {
		name: "NotReady status without reason should not be inactive",
		status: RevisionStatus{
			Status: duckv1beta1.Status{
				Conditions: duckv1beta1.Conditions{{
					Type:   RevisionConditionReady,
					Status: corev1.ConditionFalse,
				}},
			},
		},
		isActivationRequired: false,
	}, {
		name: "Ready/Unknown status without reason should not be inactive",
		status: RevisionStatus{
			Status: duckv1beta1.Status{
				Conditions: duckv1beta1.Conditions{{
					Type:   RevisionConditionReady,
					Status: corev1.ConditionUnknown,
				}},
			},
		},
		isActivationRequired: false,
	}}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if e, a := tc.isActivationRequired, tc.status.IsActivationRequired(); e != a {
				t.Errorf("%q expected: %v got: %v", tc.name, e, a)
			}
		})
	}
}

func TestIsReady(t *testing.T) {
	cases := []struct {
		name    string
		status  RevisionStatus
		isReady bool
	}{{
		name:    "empty status should not be ready",
		status:  RevisionStatus{},
		isReady: false,
	}, {
		name: "Different condition type should not be ready",
		status: RevisionStatus{
			Status: duckv1beta1.Status{
				Conditions: duckv1beta1.Conditions{{
					Type:   RevisionConditionResourcesAvailable,
					Status: corev1.ConditionTrue,
				}},
			},
		},
		isReady: false,
	}, {
		name: "False condition status should not be ready",
		status: RevisionStatus{
			Status: duckv1beta1.Status{
				Conditions: duckv1beta1.Conditions{{
					Type:   RevisionConditionReady,
					Status: corev1.ConditionFalse,
				}},
			},
		},
		isReady: false,
	}, {
		name: "Unknown condition status should not be ready",
		status: RevisionStatus{
			Status: duckv1beta1.Status{
				Conditions: duckv1beta1.Conditions{{
					Type:   RevisionConditionReady,
					Status: corev1.ConditionUnknown,
				}},
			},
		},
		isReady: false,
	}, {
		name: "Missing condition status should not be ready",
		status: RevisionStatus{
			Status: duckv1beta1.Status{
				Conditions: duckv1beta1.Conditions{{
					Type: RevisionConditionReady,
				}},
			},
		},
		isReady: false,
	}, {
		name: "True condition status should be ready",
		status: RevisionStatus{
			Status: duckv1beta1.Status{
				Conditions: duckv1beta1.Conditions{{
					Type:   RevisionConditionReady,
					Status: corev1.ConditionTrue,
				}},
			},
		},
		isReady: true,
	}, {
		name: "Multiple conditions with ready status should be ready",
		status: RevisionStatus{
			Status: duckv1beta1.Status{
				Conditions: duckv1beta1.Conditions{{
					Type:   RevisionConditionResourcesAvailable,
					Status: corev1.ConditionTrue,
				}, {
					Type:   RevisionConditionReady,
					Status: corev1.ConditionTrue,
				}},
			},
		},
		isReady: true,
	}, {
		name: "Multiple conditions with ready status false should not be ready",
		status: RevisionStatus{
			Status: duckv1beta1.Status{
				Conditions: duckv1beta1.Conditions{{
					Type:   RevisionConditionResourcesAvailable,
					Status: corev1.ConditionTrue,
				}, {
					Type:   RevisionConditionReady,
					Status: corev1.ConditionFalse,
				}},
			},
		},
		isReady: false,
	}}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if e, a := tc.isReady, tc.status.IsReady(); e != a {
				t.Errorf("%q expected: %v got: %v", tc.name, e, a)
			}
		})
	}
}

func TestGetSetCondition(t *testing.T) {
	rs := &RevisionStatus{}
	if a := rs.GetCondition(RevisionConditionReady); a != nil {
		t.Errorf("empty RevisionStatus returned %v when expected nil", a)
	}

	rc := &apis.Condition{
		Type:     RevisionConditionResourcesAvailable,
		Status:   corev1.ConditionTrue,
		Severity: apis.ConditionSeverityError,
	}

	rs.MarkResourcesAvailable()

	if diff := cmp.Diff(rc, rs.GetCondition(RevisionConditionResourcesAvailable), cmpopts.IgnoreFields(apis.Condition{}, "LastTransitionTime")); diff != "" {
		t.Errorf("GetCondition refs diff (-want +got): %v", diff)
	}
	if a := rs.GetCondition(RevisionConditionReady); a != nil {
		t.Errorf("GetCondition expected nil got: %v", a)
	}
}

func TestTypicalFlowWithProgressDeadlineExceeded(t *testing.T) {
	r := &RevisionStatus{}
	r.InitializeConditions()
	apitest.CheckConditionOngoing(r.duck(), RevisionConditionResourcesAvailable, t)
	apitest.CheckConditionOngoing(r.duck(), RevisionConditionContainerHealthy, t)
	apitest.CheckConditionOngoing(r.duck(), RevisionConditionReady, t)

	const want = "the error message"
	r.MarkProgressDeadlineExceeded(want)
	apitest.CheckConditionFailed(r.duck(), RevisionConditionResourcesAvailable, t)
	apitest.CheckConditionFailed(r.duck(), RevisionConditionReady, t)
	if got := r.GetCondition(RevisionConditionResourcesAvailable); got == nil || got.Message != want {
		t.Errorf("MarkProgressDeadlineExceeded = %v, want %v", got, want)
	}
	if got := r.GetCondition(RevisionConditionReady); got == nil || got.Message != want {
		t.Errorf("MarkProgressDeadlineExceeded = %v, want %v", got, want)
	}
}

func TestTypicalFlowWithContainerMissing(t *testing.T) {
	r := &RevisionStatus{}
	r.InitializeConditions()
	apitest.CheckConditionOngoing(r.duck(), RevisionConditionResourcesAvailable, t)
	apitest.CheckConditionOngoing(r.duck(), RevisionConditionContainerHealthy, t)
	apitest.CheckConditionOngoing(r.duck(), RevisionConditionReady, t)

	const want = "something about the container being not found"
	r.MarkContainerMissing(want)
	apitest.CheckConditionOngoing(r.duck(), RevisionConditionResourcesAvailable, t)
	apitest.CheckConditionFailed(r.duck(), RevisionConditionContainerHealthy, t)
	apitest.CheckConditionFailed(r.duck(), RouteConditionReady, t)
	if got := r.GetCondition(RevisionConditionContainerHealthy); got == nil || got.Message != want {
		t.Errorf("MarkContainerMissing = %v, want %v", got, want)
	} else if got.Reason != "ContainerMissing" {
		t.Errorf("MarkContainerMissing = %v, want %v", got, "ContainerMissing")
	}
	if got := r.GetCondition(RevisionConditionReady); got == nil || got.Message != want {
		t.Errorf("MarkContainerMissing = %v, want %v", got, want)
	} else if got.Reason != "ContainerMissing" {
		t.Errorf("MarkContainerMissing = %v, want %v", got, "ContainerMissing")
	}
}

func TestTypicalFlowWithSuspendResume(t *testing.T) {
	r := &RevisionStatus{}
	r.InitializeConditions()
	apitest.CheckConditionOngoing(r.duck(), RevisionConditionResourcesAvailable, t)
	apitest.CheckConditionOngoing(r.duck(), RevisionConditionContainerHealthy, t)
	apitest.CheckConditionOngoing(r.duck(), RevisionConditionReady, t)

	// Enter a Ready state.
	r.MarkActive()
	r.MarkContainerHealthy()
	r.MarkResourcesAvailable()
	apitest.CheckConditionSucceeded(r.duck(), RevisionConditionResourcesAvailable, t)
	apitest.CheckConditionSucceeded(r.duck(), RevisionConditionContainerHealthy, t)
	apitest.CheckConditionSucceeded(r.duck(), RevisionConditionReady, t)

	// From a Ready state, make the revision inactive to simulate scale to zero.
	const want = "Deactivated"
	r.MarkInactive(want, "Reserve")
	apitest.CheckConditionSucceeded(r.duck(), RevisionConditionResourcesAvailable, t)
	apitest.CheckConditionSucceeded(r.duck(), RevisionConditionContainerHealthy, t)
	apitest.CheckConditionFailed(r.duck(), RevisionConditionActive, t)
	if got := r.GetCondition(RevisionConditionActive); got == nil || got.Reason != want {
		t.Errorf("MarkInactive = %v, want %v", got, want)
	}
	apitest.CheckConditionSucceeded(r.duck(), RevisionConditionReady, t)

	// From an Inactive state, start to activate the revision.
	const want2 = "Activating"
	r.MarkActivating(want2, "blah blah blah")
	apitest.CheckConditionSucceeded(r.duck(), RevisionConditionResourcesAvailable, t)
	apitest.CheckConditionSucceeded(r.duck(), RevisionConditionContainerHealthy, t)
	apitest.CheckConditionOngoing(r.duck(), RevisionConditionActive, t)
	if got := r.GetCondition(RevisionConditionActive); got == nil || got.Reason != want2 {
		t.Errorf("MarkInactive = %v, want %v", got, want2)
	}
	apitest.CheckConditionSucceeded(r.duck(), RevisionConditionReady, t)

	// From the activating state, simulate the transition back to readiness.
	r.MarkActive()
	apitest.CheckConditionSucceeded(r.duck(), RevisionConditionResourcesAvailable, t)
	apitest.CheckConditionSucceeded(r.duck(), RevisionConditionContainerHealthy, t)
	apitest.CheckConditionSucceeded(r.duck(), RevisionConditionReady, t)
}

func TestRevisionNotOwnedStuff(t *testing.T) {
	r := &RevisionStatus{}
	r.InitializeConditions()
	apitest.CheckConditionOngoing(r.duck(), RevisionConditionResourcesAvailable, t)
	apitest.CheckConditionOngoing(r.duck(), RevisionConditionContainerHealthy, t)
	apitest.CheckConditionOngoing(r.duck(), RevisionConditionReady, t)

	const want = "NotOwned"
	r.MarkResourceNotOwned("Resource", "mark")
	apitest.CheckConditionFailed(r.duck(), RevisionConditionResourcesAvailable, t)
	apitest.CheckConditionFailed(r.duck(), RevisionConditionReady, t)
	if got := r.GetCondition(RevisionConditionResourcesAvailable); got == nil || got.Reason != want {
		t.Errorf("MarkResourceNotOwned = %v, want %v", got, want)
	}
	if got := r.GetCondition(RevisionConditionReady); got == nil || got.Reason != want {
		t.Errorf("MarkResourceNotOwned = %v, want %v", got, want)
	}
}

func TestRevisionResourcesUnavailable(t *testing.T) {
	r := &RevisionStatus{}
	r.InitializeConditions()
	apitest.CheckConditionOngoing(r.duck(), RevisionConditionResourcesAvailable, t)
	apitest.CheckConditionOngoing(r.duck(), RevisionConditionContainerHealthy, t)
	apitest.CheckConditionOngoing(r.duck(), RevisionConditionReady, t)

	const wantReason, wantMessage = "unschedulable", "insufficient energy"
	r.MarkResourcesUnavailable(wantReason, wantMessage)
	apitest.CheckConditionFailed(r.duck(), RevisionConditionResourcesAvailable, t)
	apitest.CheckConditionFailed(r.duck(), RevisionConditionReady, t)
	if got := r.GetCondition(RevisionConditionResourcesAvailable); got == nil || got.Reason != wantReason {
		t.Errorf("RevisionConditionResourcesAvailable = %v, want %v", got, wantReason)
	}
	if got := r.GetCondition(RevisionConditionResourcesAvailable); got == nil || got.Message != wantMessage {
		t.Errorf("RevisionConditionResourcesAvailable = %v, want %v", got, wantMessage)
	}
}

func TestRevisionGetGroupVersionKind(t *testing.T) {
	r := &Revision{}
	want := schema.GroupVersionKind{
		Group:   "serving.knative.dev",
		Version: "v1alpha1",
		Kind:    "Revision",
	}
	if got := r.GetGroupVersionKind(); got != want {
		t.Errorf("got: %v, want: %v", got, want)
	}
}

func TestRevisionBuildRefFromName(t *testing.T) {
	r := &Revision{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "foo-space",
			Name:      "foo",
		},
		Spec: RevisionSpec{
			DeprecatedBuildName: "bar-build",
		},
	}
	got := *r.DeprecatedBuildRef()
	want := corev1.ObjectReference{
		APIVersion: "build.knative.dev/v1alpha1",
		Kind:       "Build",
		Namespace:  "foo-space",
		Name:       "bar-build",
	}
	if got != want {
		t.Errorf("got: %#v, want: %#v", got, want)
	}
}

func TestRevisionBuildRef(t *testing.T) {
	buildRef := corev1.ObjectReference{
		APIVersion: "testing.build.knative.dev/v1alpha1",
		Kind:       "Build",
		Namespace:  "foo-space",
		Name:       "foo-build",
	}
	r := &Revision{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "foo-space",
			Name:      "foo",
		},
		Spec: RevisionSpec{
			DeprecatedBuildName: "bar",
			DeprecatedBuildRef:  &buildRef,
		},
	}
	got := *r.DeprecatedBuildRef()
	want := buildRef
	if got != want {
		t.Errorf("got: %#v, want: %#v", got, want)
	}
}

func TestRevisionBuildRefNil(t *testing.T) {
	r := &Revision{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "foo-space",
			Name:      "foo",
		},
	}
	got := r.DeprecatedBuildRef()

	var want *corev1.ObjectReference
	if got != want {
		t.Errorf("got: %#v, want: %#v", got, want)
	}
}

func TestRevisionGetProtocol(t *testing.T) {
	containerWithPortName := func(name string) corev1.Container {
		return corev1.Container{Ports: []corev1.ContainerPort{{Name: name}}}
	}

	tests := []struct {
		name      string
		container corev1.Container
		protocol  net.ProtocolType
	}{{
		name:      "undefined",
		container: corev1.Container{},
		protocol:  net.ProtocolHTTP1,
	}, {
		name:      "http1",
		container: containerWithPortName("http1"),
		protocol:  net.ProtocolHTTP1,
	}, {
		name:      "h2c",
		container: containerWithPortName("h2c"),
		protocol:  net.ProtocolH2C,
	}, {
		name:      "unknown",
		container: containerWithPortName("whatever"),
		protocol:  net.ProtocolHTTP1,
	}, {
		name:      "empty",
		container: containerWithPortName(""),
		protocol:  net.ProtocolHTTP1,
	}}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := &Revision{
				Spec: RevisionSpec{
					DeprecatedContainer: &tt.container,
				},
			}

			got := r.GetProtocol()
			want := tt.protocol

			if got != want {
				t.Errorf("got: %#v, want: %#v", got, want)
			}
		})
	}
}

func TestRevisionGetLastPinned(t *testing.T) {
	cases := []struct {
		name              string
		annotations       map[string]string
		expectTime        time.Time
		setLastPinnedTime time.Time
		expectErr         error
	}{{
		name:        "Nil annotations",
		annotations: nil,
		expectErr: LastPinnedParseError{
			Type: AnnotationParseErrorTypeMissing,
		},
	}, {
		name:        "Empty map annotations",
		annotations: map[string]string{},
		expectErr: LastPinnedParseError{
			Type: AnnotationParseErrorTypeMissing,
		},
	}, {
		name:              "Empty map annotations - with set time",
		annotations:       map[string]string{},
		setLastPinnedTime: time.Unix(1000, 0),
		expectTime:        time.Unix(1000, 0),
	}, {
		name:        "Invalid time",
		annotations: map[string]string{serving.RevisionLastPinnedAnnotationKey: "abcd"},
		expectErr: LastPinnedParseError{
			Type:  AnnotationParseErrorTypeInvalid,
			Value: "abcd",
		},
	}, {
		name:        "Valid time",
		annotations: map[string]string{serving.RevisionLastPinnedAnnotationKey: "10000"},
		expectTime:  time.Unix(10000, 0),
	}}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rev := Revision{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: tc.annotations,
				},
			}

			if tc.setLastPinnedTime != (time.Time{}) {
				rev.SetLastPinned(tc.setLastPinnedTime)
			}

			pt, err := rev.GetLastPinned()
			failErr := func() {
				t.Fatalf("Expected error %v got %v", tc.expectErr, err)
			}

			if tc.expectErr == nil {
				if err != nil {
					failErr()
				}
			} else {
				if tc.expectErr.Error() != err.Error() {
					failErr()
				}
			}

			if tc.expectTime != pt {
				t.Fatalf("Expected pin time %v got %v", tc.expectTime, pt)
			}
		})
	}
}

func TestRevisionIsReachable(t *testing.T) {
	tests := []struct {
		name   string
		labels map[string]string
		want   bool
	}{{
		name:   "has route annotation",
		labels: map[string]string{serving.RouteLabelKey: "the-route"},
		want:   true,
	}, {
		name:   "empty route annotation",
		labels: map[string]string{serving.RouteLabelKey: ""},
		want:   false,
	}, {
		name:   "no route annotation",
		labels: nil,
		want:   false,
	}}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rev := Revision{ObjectMeta: metav1.ObjectMeta{Labels: tt.labels}}

			got := rev.IsReachable()

			if got != tt.want {
				t.Errorf("got: %t, want: %t", got, tt.want)
			}
		})
	}
}

func TestPropagateDeploymentStatus(t *testing.T) {
	rev := &RevisionStatus{}
	rev.InitializeConditions()

	// We start out ongoing.
	apitest.CheckConditionOngoing(rev.duck(), RevisionConditionReady, t)
	apitest.CheckConditionOngoing(rev.duck(), RevisionConditionContainerHealthy, t)
	apitest.CheckConditionOngoing(rev.duck(), RevisionConditionResourcesAvailable, t)

	// Empty deployment conditions shouldn't affect our readiness.
	rev.PropagateDeploymentStatus(&appsv1.DeploymentStatus{
		Conditions: []appsv1.DeploymentCondition{},
	})
	apitest.CheckConditionOngoing(rev.duck(), RevisionConditionReady, t)
	apitest.CheckConditionOngoing(rev.duck(), RevisionConditionContainerHealthy, t)
	apitest.CheckConditionOngoing(rev.duck(), RevisionConditionResourcesAvailable, t)

	// Deployment failures should be propagated and not affect ContainerHealthy.
	rev.PropagateDeploymentStatus(&appsv1.DeploymentStatus{
		Conditions: []appsv1.DeploymentCondition{{
			Type:   appsv1.DeploymentProgressing,
			Status: corev1.ConditionFalse,
		}, {
			Type:   appsv1.DeploymentReplicaFailure,
			Status: corev1.ConditionUnknown,
		}},
	})
	apitest.CheckConditionFailed(rev.duck(), RevisionConditionReady, t)
	apitest.CheckConditionFailed(rev.duck(), RevisionConditionResourcesAvailable, t)
	apitest.CheckConditionOngoing(rev.duck(), RevisionConditionContainerHealthy, t)

	// Marking container healthy doesn't affect deployment status.
	rev.MarkContainerHealthy()
	apitest.CheckConditionFailed(rev.duck(), RevisionConditionReady, t)
	apitest.CheckConditionFailed(rev.duck(), RevisionConditionResourcesAvailable, t)
	apitest.CheckConditionSucceeded(rev.duck(), RevisionConditionContainerHealthy, t)

	// We can recover from deployment failures.
	rev.PropagateDeploymentStatus(&appsv1.DeploymentStatus{
		Conditions: []appsv1.DeploymentCondition{{
			Type:   appsv1.DeploymentProgressing,
			Status: corev1.ConditionTrue,
		}},
	})
	apitest.CheckConditionSucceeded(rev.duck(), RevisionConditionReady, t)
	apitest.CheckConditionSucceeded(rev.duck(), RevisionConditionResourcesAvailable, t)
	apitest.CheckConditionSucceeded(rev.duck(), RevisionConditionContainerHealthy, t)

	// We can go unknown.
	rev.PropagateDeploymentStatus(&appsv1.DeploymentStatus{
		Conditions: []appsv1.DeploymentCondition{{
			Type:   appsv1.DeploymentProgressing,
			Status: corev1.ConditionUnknown,
		}},
	})
	apitest.CheckConditionOngoing(rev.duck(), RevisionConditionReady, t)
	apitest.CheckConditionOngoing(rev.duck(), RevisionConditionResourcesAvailable, t)
	apitest.CheckConditionSucceeded(rev.duck(), RevisionConditionContainerHealthy, t)

	// ReplicaFailure=True translates into Ready=False.
	rev.PropagateDeploymentStatus(&appsv1.DeploymentStatus{
		Conditions: []appsv1.DeploymentCondition{{
			Type:   appsv1.DeploymentReplicaFailure,
			Status: corev1.ConditionTrue,
		}},
	})
	apitest.CheckConditionFailed(rev.duck(), RevisionConditionReady, t)
	apitest.CheckConditionFailed(rev.duck(), RevisionConditionResourcesAvailable, t)
	apitest.CheckConditionSucceeded(rev.duck(), RevisionConditionContainerHealthy, t)

	// ReplicaFailure=True trumps Progressing=Unknown.
	rev.PropagateDeploymentStatus(&appsv1.DeploymentStatus{
		Conditions: []appsv1.DeploymentCondition{{
			Type:   appsv1.DeploymentProgressing,
			Status: corev1.ConditionUnknown,
		}, {
			Type:   appsv1.DeploymentReplicaFailure,
			Status: corev1.ConditionTrue,
		}},
	})
	apitest.CheckConditionFailed(rev.duck(), RevisionConditionReady, t)
	apitest.CheckConditionFailed(rev.duck(), RevisionConditionResourcesAvailable, t)
	apitest.CheckConditionSucceeded(rev.duck(), RevisionConditionContainerHealthy, t)

	// ReplicaFailure=False + Progressing=True yields Ready.
	rev.PropagateDeploymentStatus(&appsv1.DeploymentStatus{
		Conditions: []appsv1.DeploymentCondition{{
			Type:   appsv1.DeploymentProgressing,
			Status: corev1.ConditionTrue,
		}, {
			Type:   appsv1.DeploymentReplicaFailure,
			Status: corev1.ConditionFalse,
		}},
	})
	apitest.CheckConditionSucceeded(rev.duck(), RevisionConditionReady, t)
	apitest.CheckConditionSucceeded(rev.duck(), RevisionConditionResourcesAvailable, t)
	apitest.CheckConditionSucceeded(rev.duck(), RevisionConditionContainerHealthy, t)
}
