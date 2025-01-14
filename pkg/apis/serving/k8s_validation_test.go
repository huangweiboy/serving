/*
Copyright 2019 The Knative Authors

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

package serving

import (
	"fmt"
	"math"
	"testing"

	"github.com/google/go-cmp/cmp"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/apimachinery/pkg/util/sets"
	"knative.dev/pkg/apis"
	"knative.dev/pkg/ptr"
)

func TestPodSpecValidation(t *testing.T) {
	tests := []struct {
		name string
		ps   corev1.PodSpec
		want *apis.FieldError
	}{{
		name: "valid",
		ps: corev1.PodSpec{
			Containers: []corev1.Container{{
				Image: "helloworld",
			}},
		},
		want: nil,
	}, {
		name: "with volume (ok)",
		ps: corev1.PodSpec{
			Containers: []corev1.Container{{
				Image: "helloworld",
				VolumeMounts: []corev1.VolumeMount{{
					MountPath: "/mount/path",
					Name:      "the-name",
					ReadOnly:  true,
				}},
			}},
			Volumes: []corev1.Volume{{
				Name: "the-name",
				VolumeSource: corev1.VolumeSource{
					Secret: &corev1.SecretVolumeSource{
						SecretName: "foo",
					},
				},
			}},
		},
		want: nil,
	}, {
		name: "with volume name collision",
		ps: corev1.PodSpec{
			Containers: []corev1.Container{{
				Image: "helloworld",
				VolumeMounts: []corev1.VolumeMount{{
					MountPath: "/mount/path",
					Name:      "the-name",
					ReadOnly:  true,
				}},
			}},
			Volumes: []corev1.Volume{{
				Name: "the-name",
				VolumeSource: corev1.VolumeSource{
					Secret: &corev1.SecretVolumeSource{
						SecretName: "foo",
					},
				},
			}, {
				Name: "the-name",
				VolumeSource: corev1.VolumeSource{
					ConfigMap: &corev1.ConfigMapVolumeSource{},
				},
			}},
		},
		want: (&apis.FieldError{
			Message: fmt.Sprintf(`duplicate volume name "the-name"`),
			Paths:   []string{"name"},
		}).ViaFieldIndex("volumes", 1),
	}, {
		name: "with volume mount path collision",
		ps: corev1.PodSpec{
			Containers: []corev1.Container{{
				Image: "helloworld",
				VolumeMounts: []corev1.VolumeMount{{
					MountPath: "/mount/path",
					Name:      "the-foo",
					ReadOnly:  true,
				}, {
					MountPath: "/mount/path",
					Name:      "the-bar",
					ReadOnly:  true,
				}},
			}},
			Volumes: []corev1.Volume{{
				Name: "the-foo",
				VolumeSource: corev1.VolumeSource{
					Secret: &corev1.SecretVolumeSource{
						SecretName: "foo",
					},
				},
			}, {
				Name: "the-bar",
				VolumeSource: corev1.VolumeSource{
					Secret: &corev1.SecretVolumeSource{
						SecretName: "bar",
					},
				},
			}},
		},
		want: apis.ErrInvalidValue(`"/mount/path" must be unique`, "mountPath").
			ViaFieldIndex("volumeMounts", 1).ViaFieldIndex("containers", 0),
	}, {
		name: "bad pod spec",
		ps: corev1.PodSpec{
			Containers: []corev1.Container{{
				Name:      "steve",
				Image:     "helloworld",
				Lifecycle: &corev1.Lifecycle{},
			}},
		},
		want: apis.ErrDisallowedFields("containers[0].lifecycle"),
	}, {
		name: "missing all",
		ps: corev1.PodSpec{
			Containers: []corev1.Container{},
		},
		want: apis.ErrMissingField("containers"),
	}, {
		name: "missing container",
		ps: corev1.PodSpec{
			ServiceAccountName: "bob",
			Containers:         []corev1.Container{},
		},
		want: apis.ErrMissingField("containers"),
	}, {
		name: "too many containers",
		ps: corev1.PodSpec{
			Containers: []corev1.Container{{
				Image: "busybox",
			}, {
				Image: "helloworld",
			}},
		},
		want: apis.ErrMultipleOneOf("containers"),
	}, {
		name: "extra field",
		ps: corev1.PodSpec{
			Containers: []corev1.Container{{
				Image: "busybox",
			}},
			InitContainers: []corev1.Container{{
				Image: "helloworld",
			}},
		},
		want: apis.ErrDisallowedFields("initContainers"),
	}, {
		name: "bad service account name",
		ps: corev1.PodSpec{
			Containers: []corev1.Container{{
				Image: "busybox",
			}},
			ServiceAccountName: "foo@bar.baz",
		},
		want: apis.ErrInvalidValue("serviceAccountName", "foo@bar.baz"),
	}}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := ValidatePodSpec(test.ps)
			if !cmp.Equal(test.want.Error(), got.Error()) {
				t.Errorf("ValidatePodSpec (-want, +got) = %v",
					cmp.Diff(test.want.Error(), got.Error()))
			}
		})
	}
}

func TestContainerValidation(t *testing.T) {
	bidir := corev1.MountPropagationBidirectional

	tests := []struct {
		name    string
		c       corev1.Container
		want    *apis.FieldError
		volumes sets.String
	}{{
		name: "empty container",
		c:    corev1.Container{},
		want: apis.ErrMissingField(apis.CurrentField),
	}, {
		name: "valid container",
		c: corev1.Container{
			Image: "foo",
		},
		want: nil,
	}, {
		name: "invalid container image",
		c: corev1.Container{
			Image: "foo:bar:baz",
		},
		want: &apis.FieldError{
			Message: "Failed to parse image reference",
			Paths:   []string{"image"},
			Details: "image: \"foo:bar:baz\", error: could not parse reference",
		},
	}, {
		name: "has a lifecycle",
		c: corev1.Container{
			Name:      "foo",
			Image:     "foo",
			Lifecycle: &corev1.Lifecycle{},
		},
		want: apis.ErrDisallowedFields("lifecycle"),
	}, {
		name: "has resources",
		c: corev1.Container{
			Image: "foo",
			Resources: corev1.ResourceRequirements{
				Limits: corev1.ResourceList{
					corev1.ResourceName("memory"): resource.MustParse("250M"),
				},
				Requests: corev1.ResourceList{
					corev1.ResourceName("cpu"): resource.MustParse("25m"),
				},
			},
		},
		want: nil,
	}, {
		name: "has no container ports set",
		c: corev1.Container{
			Image: "foo",
			Ports: []corev1.ContainerPort{},
		},
		want: nil,
	}, {
		name: "has valid unnamed user port",
		c: corev1.Container{
			Image: "foo",
			Ports: []corev1.ContainerPort{{
				ContainerPort: 8181,
			}},
		},
		want: nil,
	}, {
		name: "has valid user port http1",
		c: corev1.Container{
			Image: "foo",
			Ports: []corev1.ContainerPort{{
				Name: "http1",
			}},
		},
		want: nil,
	}, {
		name: "has valid user port h2c",
		c: corev1.Container{
			Image: "foo",
			Ports: []corev1.ContainerPort{{
				Name: "h2c",
			}},
		},
		want: nil,
	}, {
		name: "has more than one ports with valid names",
		c: corev1.Container{
			Image: "foo",
			Ports: []corev1.ContainerPort{{
				Name: "h2c",
			}, {
				Name: "http1",
			}},
		},
		want: &apis.FieldError{
			Message: "More than one container port is set",
			Paths:   []string{"ports"},
			Details: "Only a single port is allowed",
		},
	}, {
		name: "has container port value too large",
		c: corev1.Container{
			Image: "foo",
			Ports: []corev1.ContainerPort{{
				ContainerPort: 65536,
			}},
		},
		want: apis.ErrOutOfBoundsValue(65536, 0, 65535, "ports.containerPort"),
	}, {
		name: "has an empty port set",
		c: corev1.Container{
			Image: "foo",
			Ports: []corev1.ContainerPort{{}},
		},
		want: nil,
	}, {
		name: "has more than one unnamed port",
		c: corev1.Container{
			Image: "foo",
			Ports: []corev1.ContainerPort{{
				ContainerPort: 8080,
			}, {
				ContainerPort: 8181,
			}},
		},
		want: &apis.FieldError{
			Message: "More than one container port is set",
			Paths:   []string{"ports"},
			Details: "Only a single port is allowed",
		},
	}, {
		name: "has tcp protocol",
		c: corev1.Container{
			Image: "foo",
			Ports: []corev1.ContainerPort{{
				Protocol: corev1.ProtocolTCP,
			}},
		},
		want: nil,
	}, {
		name: "has invalid protocol",
		c: corev1.Container{
			Image: "foo",
			Ports: []corev1.ContainerPort{{
				Protocol: "tdp",
			}},
		},
		want: apis.ErrInvalidValue("tdp", "ports.protocol"),
	}, {
		name: "has host port",
		c: corev1.Container{
			Image: "foo",
			Ports: []corev1.ContainerPort{{
				HostPort: 80,
			}},
		},
		want: apis.ErrDisallowedFields("ports.hostPort"),
	}, {
		name: "has host ip",
		c: corev1.Container{
			Image: "foo",
			Ports: []corev1.ContainerPort{{
				HostIP: "127.0.0.1",
			}},
		},
		want: apis.ErrDisallowedFields("ports.hostIP"),
	}, {
		name: "port conflicts with queue proxy admin",
		c: corev1.Container{
			Image: "foo",
			Ports: []corev1.ContainerPort{{
				ContainerPort: 8022,
			}},
		},
		want: apis.ErrInvalidValue(8022, "ports.containerPort"),
	}, {
		name: "port conflicts with queue proxy",
		c: corev1.Container{
			Image: "foo",
			Ports: []corev1.ContainerPort{{
				ContainerPort: 8013,
			}},
		},
		want: apis.ErrInvalidValue(8013, "ports.containerPort"),
	}, {
		name: "port conflicts with queue proxy",
		c: corev1.Container{
			Image: "foo",
			Ports: []corev1.ContainerPort{{
				ContainerPort: 8012,
			}},
		},
		want: apis.ErrInvalidValue(8012, "ports.containerPort"),
	}, {
		name: "port conflicts with queue proxy metrics",
		c: corev1.Container{
			Image: "foo",
			Ports: []corev1.ContainerPort{{
				ContainerPort: 9090,
			}},
		},
		want: apis.ErrInvalidValue(9090, "ports.containerPort"),
	}, {
		name: "has invalid port name",
		c: corev1.Container{
			Image: "foo",
			Ports: []corev1.ContainerPort{{
				Name: "foobar",
			}},
		},
		want: &apis.FieldError{
			Message: fmt.Sprintf("Port name %v is not allowed", "foobar"),
			Paths:   []string{"ports"},
			Details: "Name must be empty, or one of: 'h2c', 'http1'",
		},
	}, {
		name: "has unknown volumeMounts",
		c: corev1.Container{
			Image: "foo",
			VolumeMounts: []corev1.VolumeMount{{
				Name:             "the-name",
				SubPath:          "oops",
				MountPropagation: &bidir,
			}},
		},
		want: (&apis.FieldError{
			Message: "volumeMount has no matching volume",
			Paths:   []string{"name"},
		}).ViaFieldIndex("volumeMounts", 0).Also(
			apis.ErrMissingField("readOnly").ViaFieldIndex("volumeMounts", 0)).Also(
			apis.ErrMissingField("mountPath").ViaFieldIndex("volumeMounts", 0)).Also(
			apis.ErrDisallowedFields("mountPropagation").ViaFieldIndex("volumeMounts", 0)),
	}, {
		name: "missing known volumeMounts",
		c: corev1.Container{
			Image: "foo",
		},
		volumes: sets.NewString("the-name"),
		want: &apis.FieldError{
			Message: "volumes not mounted: [the-name]",
			Paths:   []string{"volumeMounts"},
		},
	}, {
		name: "has known volumeMounts",
		c: corev1.Container{
			Image: "foo",
			VolumeMounts: []corev1.VolumeMount{{
				MountPath: "/mount/path",
				Name:      "the-name",
				ReadOnly:  true,
			}},
		},
		volumes: sets.NewString("the-name"),
	}, {
		name: "has known volumeMounts, but at reserved path",
		c: corev1.Container{
			Image: "foo",
			VolumeMounts: []corev1.VolumeMount{{
				MountPath: "//var//log//",
				Name:      "the-name",
				ReadOnly:  true,
			}},
		},
		volumes: sets.NewString("the-name"),
		want: (&apis.FieldError{
			Message: `mountPath "/var/log" is a reserved path`,
			Paths:   []string{"mountPath"},
		}).ViaFieldIndex("volumeMounts", 0),
	}, {
		name: "has known volumeMounts, bad mountPath",
		c: corev1.Container{
			Image: "foo",
			VolumeMounts: []corev1.VolumeMount{{
				MountPath: "not/absolute",
				Name:      "the-name",
				ReadOnly:  true,
			}},
		},
		volumes: sets.NewString("the-name"),
		want:    apis.ErrInvalidValue("not/absolute", "volumeMounts[0].mountPath"),
	}, {
		name: "has lifecycle",
		c: corev1.Container{
			Image:     "foo",
			Lifecycle: &corev1.Lifecycle{},
		},
		want: apis.ErrDisallowedFields("lifecycle"),
	}, {
		name: "has known volumeMount twice",
		c: corev1.Container{
			Image: "foo",
			VolumeMounts: []corev1.VolumeMount{{
				MountPath: "/mount/path",
				Name:      "the-name",
				ReadOnly:  true,
			}, {
				MountPath: "/another/mount/path",
				Name:      "the-name",
				ReadOnly:  true,
			}},
		},
		volumes: sets.NewString("the-name"),
	}, {
		name: "valid with probes (no port)",
		c: corev1.Container{
			Image: "foo",
			ReadinessProbe: &corev1.Probe{
				PeriodSeconds:    1,
				TimeoutSeconds:   1,
				SuccessThreshold: 1,
				FailureThreshold: 3,
				Handler: corev1.Handler{
					HTTPGet: &corev1.HTTPGetAction{
						Path: "/",
					},
				},
			},
			LivenessProbe: &corev1.Probe{
				Handler: corev1.Handler{
					TCPSocket: &corev1.TCPSocketAction{},
				},
			},
		},
		want: nil,
	}, {
		name: "valid with exec probes ",
		c: corev1.Container{
			Image: "foo",
			ReadinessProbe: &corev1.Probe{
				PeriodSeconds:    1,
				TimeoutSeconds:   1,
				SuccessThreshold: 1,
				FailureThreshold: 3,
				Handler: corev1.Handler{
					HTTPGet: &corev1.HTTPGetAction{
						Path: "/",
					},
				},
			},
			LivenessProbe: &corev1.Probe{
				Handler: corev1.Handler{
					Exec: &corev1.ExecAction{},
				},
			},
		},
		want: nil,
	}, {
		name: "invalid with no handler",
		c: corev1.Container{
			Image: "foo",
			ReadinessProbe: &corev1.Probe{
				PeriodSeconds:    1,
				TimeoutSeconds:   1,
				SuccessThreshold: 1,
				FailureThreshold: 3,
				Handler: corev1.Handler{
					HTTPGet: &corev1.HTTPGetAction{
						Path: "/",
					},
				},
			},
			LivenessProbe: &corev1.Probe{
				Handler: corev1.Handler{},
			},
		},
		want: apis.ErrMissingField("livenessProbe.handler"),
	}, {
		name: "invalid with multiple handlers",
		c: corev1.Container{
			Image: "foo",
			ReadinessProbe: &corev1.Probe{
				PeriodSeconds:    1,
				TimeoutSeconds:   1,
				SuccessThreshold: 1,
				FailureThreshold: 3,
				Handler: corev1.Handler{
					HTTPGet: &corev1.HTTPGetAction{
						Path: "/",
					},
					Exec:      &corev1.ExecAction{},
					TCPSocket: &corev1.TCPSocketAction{},
				},
			},
		},
		want: apis.ErrMultipleOneOf("readinessProbe.exec", "readinessProbe.tcpSocket", "readinessProbe.httpGet"),
	}, {
		name: "invalid readiness http probe (has port)",
		c: corev1.Container{
			Image: "foo",
			ReadinessProbe: &corev1.Probe{
				PeriodSeconds:    1,
				TimeoutSeconds:   1,
				SuccessThreshold: 1,
				FailureThreshold: 3,
				Handler: corev1.Handler{
					HTTPGet: &corev1.HTTPGetAction{
						Path: "/",
						Port: intstr.FromInt(8080),
					},
				},
			},
		},
		want: apis.ErrDisallowedFields("readinessProbe.httpGet.port"),
	}, {
		name: "invalid readiness probe (has failureThreshold while using special probe)",
		c: corev1.Container{
			Image: "foo",
			ReadinessProbe: &corev1.Probe{
				PeriodSeconds:    0,
				FailureThreshold: 2,
				SuccessThreshold: 1,
				Handler: corev1.Handler{
					HTTPGet: &corev1.HTTPGetAction{
						Path: "/",
					},
				},
			},
		},
		want: apis.ErrDisallowedFields("readinessProbe.failureThreshold"),
	}, {
		name: "invalid readiness probe (has timeoutSeconds while using special probe)",
		c: corev1.Container{
			Image: "foo",
			ReadinessProbe: &corev1.Probe{
				PeriodSeconds:    0,
				TimeoutSeconds:   2,
				SuccessThreshold: 1,
				Handler: corev1.Handler{
					HTTPGet: &corev1.HTTPGetAction{
						Path: "/",
					},
				},
			},
		},
		want: apis.ErrDisallowedFields("readinessProbe.timeoutSeconds"),
	}, {
		name: "out of bounds probe values",
		c: corev1.Container{
			Image: "foo",
			ReadinessProbe: &corev1.Probe{
				PeriodSeconds:    -1,
				TimeoutSeconds:   0,
				SuccessThreshold: 0,
				FailureThreshold: 0,
				Handler: corev1.Handler{
					HTTPGet: &corev1.HTTPGetAction{},
				},
			},
		},
		want: apis.ErrOutOfBoundsValue(-1, 0, math.MaxInt32, "readinessProbe.periodSeconds").Also(
			apis.ErrOutOfBoundsValue(0, 1, math.MaxInt32, "readinessProbe.timeoutSeconds")).Also(
			apis.ErrOutOfBoundsValue(0, 1, math.MaxInt32, "readinessProbe.successThreshold")).Also(
			apis.ErrOutOfBoundsValue(0, 1, math.MaxInt32, "readinessProbe.failureThreshold")),
	}, {
		name: "disallowed security context field",
		c: corev1.Container{
			Image: "foo",
			SecurityContext: &corev1.SecurityContext{
				RunAsGroup: ptr.Int64(10),
			},
		},
		want: apis.ErrDisallowedFields("securityContext.runAsGroup"),
	}, {
		name: "too large uid",
		c: corev1.Container{
			Image: "foo",
			SecurityContext: &corev1.SecurityContext{
				RunAsUser: ptr.Int64(math.MaxInt32 + 1),
			},
		},
		want: apis.ErrOutOfBoundsValue(int64(math.MaxInt32+1), 0, math.MaxInt32, "securityContext.runAsUser"),
	}, {
		name: "negative uid",
		c: corev1.Container{
			Image: "foo",
			SecurityContext: &corev1.SecurityContext{
				RunAsUser: ptr.Int64(-10),
			},
		},
		want: apis.ErrOutOfBoundsValue(-10, 0, math.MaxInt32, "securityContext.runAsUser"),
	}, {
		name: "envFrom - None of",
		c: corev1.Container{
			Image:   "foo",
			EnvFrom: []corev1.EnvFromSource{{}},
		},
		want: apis.ErrMissingOneOf("envFrom.configMapRef", "envFrom.secretRef"),
	}, {
		name: "envFrom - Multiple",
		c: corev1.Container{
			Image: "foo",
			EnvFrom: []corev1.EnvFromSource{{
				ConfigMapRef: &corev1.ConfigMapEnvSource{
					LocalObjectReference: corev1.LocalObjectReference{
						Name: "ConfigMapName",
					},
				},
				SecretRef: &corev1.SecretEnvSource{
					LocalObjectReference: corev1.LocalObjectReference{
						Name: "SecretName",
					},
				},
			}},
		},
		want: apis.ErrMultipleOneOf("envFrom.configMapRef", "envFrom.secretRef"),
	}, {
		name: "envFrom - Secret",
		c: corev1.Container{
			Image: "foo",
			EnvFrom: []corev1.EnvFromSource{{
				SecretRef: &corev1.SecretEnvSource{
					LocalObjectReference: corev1.LocalObjectReference{
						Name: "SecretName",
					},
				},
			}},
		},
		want: nil,
	}, {
		name: "envFrom - ConfigMap",
		c: corev1.Container{
			Image: "foo",
			EnvFrom: []corev1.EnvFromSource{{
				ConfigMapRef: &corev1.ConfigMapEnvSource{
					LocalObjectReference: corev1.LocalObjectReference{
						Name: "ConfigMapName",
					},
				},
			}},
		},
		want: nil,
	}, {
		name: "termination message policy",
		c: corev1.Container{
			Image:                    "foo",
			TerminationMessagePolicy: corev1.TerminationMessagePolicy("Not a Policy"),
		},
		want: apis.ErrInvalidValue(corev1.TerminationMessagePolicy("Not a Policy"), "terminationMessagePolicy"),
	}, {
		name: "empty env var name",
		c: corev1.Container{
			Image: "foo",
			Env: []corev1.EnvVar{{
				Value: "Foo",
			}},
		},
		want: apis.ErrMissingField("env[0].name"),
	}, {
		name: "reserved env var name",
		c: corev1.Container{
			Image: "foo",
			Env: []corev1.EnvVar{{
				Name:  "PORT",
				Value: "Foo",
			}},
		},
		want: &apis.FieldError{
			Message: `"PORT" is a reserved environment variable`,
			Paths:   []string{"env[0].name"},
		},
	}, {
		name: "disallowed envvarsource",
		c: corev1.Container{
			Image: "foo",
			Env: []corev1.EnvVar{{
				Name: "Foo",
				ValueFrom: &corev1.EnvVarSource{
					FieldRef: &corev1.ObjectFieldSelector{
						FieldPath: "/v1",
					},
				},
			}},
		},
		want: apis.ErrDisallowedFields("env[0].valueFrom.fieldRef"),
	}, {
		name: "invalid liveness tcp probe (has port)",
		c: corev1.Container{
			Image: "foo",
			LivenessProbe: &corev1.Probe{
				Handler: corev1.Handler{
					TCPSocket: &corev1.TCPSocketAction{
						Port: intstr.FromString("http"),
					},
				},
			},
		},
		want: apis.ErrDisallowedFields("livenessProbe.tcpSocket.port"),
	}, {
		name: "disallowed container fields",
		c: corev1.Container{
			Image:     "foo",
			Name:      "fail",
			Stdin:     true,
			StdinOnce: true,
			TTY:       true,
			Lifecycle: &corev1.Lifecycle{},
			VolumeDevices: []corev1.VolumeDevice{{
				Name:       "disallowed",
				DevicePath: "/",
			}},
		},
		want: apis.ErrDisallowedFields("lifecycle").Also(
			apis.ErrDisallowedFields("stdin")).Also(
			apis.ErrDisallowedFields("stdinOnce")).Also(
			apis.ErrDisallowedFields("tty")).Also(
			apis.ErrDisallowedFields("volumeDevices")),
	}, {
		name: "has numerous problems",
		c: corev1.Container{
			Lifecycle: &corev1.Lifecycle{},
		},
		want: apis.ErrDisallowedFields("lifecycle").Also(
			apis.ErrMissingField("image")),
	}}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := ValidateContainer(test.c, test.volumes)
			if diff := cmp.Diff(test.want.Error(), got.Error()); diff != "" {
				t.Errorf("ValidateContainer (-want, +got) = %v", diff)
			}
		})
	}
}

func TestVolumeValidation(t *testing.T) {
	tests := []struct {
		name string
		v    corev1.Volume
		want *apis.FieldError
	}{{
		name: "just name",
		v: corev1.Volume{
			Name: "foo",
		},
		want: apis.ErrMissingOneOf("secret", "configMap", "projected"),
	}, {
		name: "secret volume",
		v: corev1.Volume{
			Name: "foo",
			VolumeSource: corev1.VolumeSource{
				Secret: &corev1.SecretVolumeSource{
					SecretName: "foo",
				},
			},
		},
	}, {
		name: "configMap volume",
		v: corev1.Volume{
			Name: "foo",
			VolumeSource: corev1.VolumeSource{
				ConfigMap: &corev1.ConfigMapVolumeSource{
					LocalObjectReference: corev1.LocalObjectReference{Name: "foo"},
				},
			},
		},
	}, {
		name: "emptyDir volume",
		v: corev1.Volume{
			Name: "foo",
			VolumeSource: corev1.VolumeSource{
				EmptyDir: &corev1.EmptyDirVolumeSource{},
			},
		},
		want: apis.ErrMissingOneOf("secret", "configMap", "projected").Also(
			apis.ErrDisallowedFields("emptyDir")),
	}, {
		name: "no volume source",
		v: corev1.Volume{
			Name: "foo",
		},
		want: apis.ErrMissingOneOf("secret", "configMap", "projected"),
	}, {
		name: "multiple volume source",
		v: corev1.Volume{
			Name: "foo",
			VolumeSource: corev1.VolumeSource{
				ConfigMap: &corev1.ConfigMapVolumeSource{
					LocalObjectReference: corev1.LocalObjectReference{Name: "foo"},
				},
				Projected: &corev1.ProjectedVolumeSource{
					Sources: []corev1.VolumeProjection{{
						Secret: &corev1.SecretProjection{
							LocalObjectReference: corev1.LocalObjectReference{
								Name: "foo",
							},
							Items: []corev1.KeyToPath{{
								Key:  "foo",
								Path: "bar/baz",
							}},
						},
					}},
				},
			},
		},
		want: apis.ErrMultipleOneOf("configMap", "projected"),
	}, {
		name: "multiple project volume single source",
		v: corev1.Volume{
			Name: "foo",
			VolumeSource: corev1.VolumeSource{
				Projected: &corev1.ProjectedVolumeSource{
					Sources: []corev1.VolumeProjection{{
						Secret: &corev1.SecretProjection{
							LocalObjectReference: corev1.LocalObjectReference{
								Name: "foo",
							},
						},
						ConfigMap: &corev1.ConfigMapProjection{
							LocalObjectReference: corev1.LocalObjectReference{
								Name: "bar",
							},
							Items: []corev1.KeyToPath{{
								Key:  "foo",
								Path: "bar/baz",
							}},
						},
					}},
				},
			},
		},
		want: apis.ErrMultipleOneOf("projected[0].configMap", "projected[0].secret"),
	}, {
		name: "multiple project volume one-per-source",
		v: corev1.Volume{
			Name: "foo",
			VolumeSource: corev1.VolumeSource{
				Projected: &corev1.ProjectedVolumeSource{
					Sources: []corev1.VolumeProjection{{
						Secret: &corev1.SecretProjection{
							LocalObjectReference: corev1.LocalObjectReference{
								Name: "foo",
							},
						},
					}, {
						ConfigMap: &corev1.ConfigMapProjection{
							LocalObjectReference: corev1.LocalObjectReference{
								Name: "bar",
							},
						},
					}},
				},
			},
		},
		want: nil,
	}, {
		name: "multiple project volume one-per-source (no names)",
		v: corev1.Volume{
			Name: "foo",
			VolumeSource: corev1.VolumeSource{
				Projected: &corev1.ProjectedVolumeSource{
					Sources: []corev1.VolumeProjection{{
						Secret: &corev1.SecretProjection{},
					}, {
						ConfigMap: &corev1.ConfigMapProjection{},
					}},
				},
			},
		},
		want: apis.ErrMissingField("projected[0].secret.name", "projected[1].configMap.name"),
	}, {
		name: "no project volume source",
		v: corev1.Volume{
			Name: "foo",
			VolumeSource: corev1.VolumeSource{
				Projected: &corev1.ProjectedVolumeSource{
					Sources: []corev1.VolumeProjection{{}},
				},
			},
		},
		want: apis.ErrMissingOneOf("projected[0].configMap", "projected[0].secret"),
	}, {
		name: "no name",
		v: corev1.Volume{
			VolumeSource: corev1.VolumeSource{
				Secret: &corev1.SecretVolumeSource{
					SecretName: "foo",
				},
			},
		},
		want: apis.ErrMissingField("name"),
	}, {
		name: "bad name",
		v: corev1.Volume{
			Name: "@@@",
			VolumeSource: corev1.VolumeSource{
				Secret: &corev1.SecretVolumeSource{
					SecretName: "foo",
				},
			},
		},
		want: apis.ErrInvalidValue("@@@", "name"),
	}, {
		name: "secret missing keyToPath values",
		v: corev1.Volume{
			Name: "foo",
			VolumeSource: corev1.VolumeSource{
				Secret: &corev1.SecretVolumeSource{
					SecretName: "foo",
					Items:      []corev1.KeyToPath{{}},
				},
			},
		},
		want: apis.ErrMissingField("items[0].key").Also(apis.ErrMissingField("items[0].path")),
	}, {
		name: "configMap missing keyToPath values",
		v: corev1.Volume{
			Name: "foo",
			VolumeSource: corev1.VolumeSource{
				ConfigMap: &corev1.ConfigMapVolumeSource{
					LocalObjectReference: corev1.LocalObjectReference{
						Name: "foo",
					},
					Items: []corev1.KeyToPath{{}},
				},
			},
		},
		want: apis.ErrMissingField("items[0].key").Also(apis.ErrMissingField("items[0].path")),
	}, {
		name: "projection missing keyToPath values",
		v: corev1.Volume{
			Name: "foo",
			VolumeSource: corev1.VolumeSource{
				Projected: &corev1.ProjectedVolumeSource{
					Sources: []corev1.VolumeProjection{{
						Secret: &corev1.SecretProjection{
							LocalObjectReference: corev1.LocalObjectReference{
								Name: "foo",
							},
							Items: []corev1.KeyToPath{{}},
						}}, {
						ConfigMap: &corev1.ConfigMapProjection{
							LocalObjectReference: corev1.LocalObjectReference{
								Name: "foo",
							},
							Items: []corev1.KeyToPath{{}},
						}},
					},
				},
			},
		},
		want: apis.ErrMissingField("projected[0].secret.items[0].key").Also(
			apis.ErrMissingField("projected[0].secret.items[0].path")).Also(
			apis.ErrMissingField("projected[1].configMap.items[0].key")).Also(
			apis.ErrMissingField("projected[1].configMap.items[0].path")),
	}}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := validateVolume(test.v)
			if diff := cmp.Diff(test.want.Error(), got.Error()); diff != "" {
				t.Errorf("validateVolume (-want, +got) = %v", diff)
			}
		})
	}
}

func TestObjectReferenceValidation(t *testing.T) {
	tests := []struct {
		name string
		r    *corev1.ObjectReference
		want *apis.FieldError
	}{{
		name: "nil",
	}, {
		name: "no api version",
		r: &corev1.ObjectReference{
			Kind: "Bar",
			Name: "foo",
		},
		want: apis.ErrMissingField("apiVersion"),
	}, {
		name: "bad api version",
		r: &corev1.ObjectReference{
			APIVersion: "/v1alpha1",
			Kind:       "Bar",
			Name:       "foo",
		},
		want: apis.ErrInvalidValue("prefix part must be non-empty", "apiVersion"),
	}, {
		name: "no kind",
		r: &corev1.ObjectReference{
			APIVersion: "foo/v1alpha1",
			Name:       "foo",
		},
		want: apis.ErrMissingField("kind"),
	}, {
		name: "bad kind",
		r: &corev1.ObjectReference{
			APIVersion: "foo/v1alpha1",
			Kind:       "Bad Kind",
			Name:       "foo",
		},
		want: apis.ErrInvalidValue("a valid C identifier must start with alphabetic character or '_', followed by a string of alphanumeric characters or '_' (e.g. 'my_name',  or 'MY_NAME',  or 'MyName', regex used for validation is '[A-Za-z_][A-Za-z0-9_]*')", "kind"),
	}, {
		name: "no namespace",
		r: &corev1.ObjectReference{
			APIVersion: "foo.group/v1alpha1",
			Kind:       "Bar",
			Name:       "the-bar-0001",
		},
		want: nil,
	}, {
		name: "no name",
		r: &corev1.ObjectReference{
			APIVersion: "foo.group/v1alpha1",
			Kind:       "Bar",
		},
		want: apis.ErrMissingField("name"),
	}, {
		name: "bad name",
		r: &corev1.ObjectReference{
			APIVersion: "foo.group/v1alpha1",
			Kind:       "Bar",
			Name:       "bad name",
		},
		want: apis.ErrInvalidValue("a DNS-1123 label must consist of lower case alphanumeric characters or '-', and must start and end with an alphanumeric character (e.g. 'my-name',  or '123-abc', regex used for validation is '[a-z0-9]([-a-z0-9]*[a-z0-9])?')", "name"),
	}, {
		name: "disallowed fields",
		r: &corev1.ObjectReference{
			APIVersion: "foo.group/v1alpha1",
			Kind:       "Bar",
			Name:       "bar0001",

			// None of these are allowed.
			Namespace:       "foo",
			FieldPath:       "some.field.path",
			ResourceVersion: "234234",
			UID:             "deadbeefcafebabe",
		},
		want: apis.ErrDisallowedFields("namespace", "fieldPath", "resourceVersion", "uid"),
	}, {
		name: "all good",
		r: &corev1.ObjectReference{
			APIVersion: "foo.group/v1alpha1",
			Kind:       "Bar",
			Name:       "bar0001",
		},
		want: nil,
	}}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := ValidateNamespacedObjectReference(test.r)
			if diff := cmp.Diff(test.want.Error(), got.Error()); diff != "" {
				t.Errorf("ValidateNamespacedObjectReference (-want, +got) = %v", diff)
			}
		})
	}
}
