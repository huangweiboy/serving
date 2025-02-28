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
	"context"
	"fmt"

	"k8s.io/apimachinery/pkg/api/equality"
	"knative.dev/pkg/apis"
	"knative.dev/serving/pkg/apis/autoscaling"
	"knative.dev/serving/pkg/apis/serving"
)

func (pa *PodAutoscaler) Validate(ctx context.Context) *apis.FieldError {
	errs := serving.ValidateObjectMetadata(pa.GetObjectMeta()).ViaField("metadata")
	errs = errs.Also(pa.validateMetric())
	return errs.Also(pa.Spec.Validate(apis.WithinSpec(ctx)).ViaField("spec"))
}

// Validate validates PodAutoscaler Spec.
func (pa *PodAutoscalerSpec) Validate(ctx context.Context) *apis.FieldError {
	if equality.Semantic.DeepEqual(pa, &PodAutoscalerSpec{}) {
		return apis.ErrMissingField(apis.CurrentField)
	}
	return serving.ValidateNamespacedObjectReference(&pa.ScaleTargetRef).ViaField("scaleTargetRef").Also(serving.ValidateContainerConcurrency(ctx, &pa.ContainerConcurrency).ViaField("containerConcurrency")).Also(validateSKSFields(ctx, pa))
}

func validateSKSFields(ctx context.Context, rs *PodAutoscalerSpec) (errs *apis.FieldError) {
	return errs.Also(rs.ProtocolType.Validate(ctx)).ViaField("protocolType")
}

func (pa *PodAutoscaler) validateMetric() *apis.FieldError {
	if metric, ok := pa.Annotations[autoscaling.MetricAnnotationKey]; ok {
		switch pa.Class() {
		case autoscaling.KPA:
			switch metric {
			case autoscaling.Concurrency, autoscaling.RPS:
				return nil
			}
		case autoscaling.HPA:
			switch metric {
			// TODO(yanweiguo): implement RPS autoscaling for HPA.
			case autoscaling.CPU, autoscaling.Concurrency:
				return nil
			}
		default:
			// Leave other classes of PodAutoscaler alone.
			return nil
		}
		return &apis.FieldError{
			Message: fmt.Sprintf("Unsupported metric %q for PodAutoscaler class %q",
				metric, pa.Class()),
			Paths: []string{"annotations[autoscaling.knative.dev/metric]"},
		}
	}
	return nil
}
