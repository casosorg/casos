package object

import (
	"reflect"
	"testing"

	corev1 "k8s.io/api/core/v1"
)

func TestContainerPortsFromPod(t *testing.T) {
	tests := []struct {
		name string
		pod  *corev1.Pod
		want []int32
	}{
		{name: "nil pod", pod: nil, want: nil},
		{name: "no containers", pod: &corev1.Pod{}, want: nil},
		{
			name: "single container single port",
			pod: &corev1.Pod{Spec: corev1.PodSpec{Containers: []corev1.Container{
				{Ports: []corev1.ContainerPort{{ContainerPort: 5800}}},
			}}},
			want: []int32{5800},
		},
		{
			name: "multi container, multi port, sorted and deduped",
			pod: &corev1.Pod{Spec: corev1.PodSpec{Containers: []corev1.Container{
				{Ports: []corev1.ContainerPort{{ContainerPort: 8080}, {ContainerPort: 5800}}},
				{Ports: []corev1.ContainerPort{{ContainerPort: 5800}, {ContainerPort: 443}}},
			}}},
			want: []int32{443, 5800, 8080},
		},
		{
			name: "port 0 ignored",
			pod: &corev1.Pod{Spec: corev1.PodSpec{Containers: []corev1.Container{
				{Ports: []corev1.ContainerPort{{ContainerPort: 0}, {ContainerPort: 80}}},
			}}},
			want: []int32{80},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ContainerPortsFromPod(tt.pod)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("got %v, want %v", got, tt.want)
			}
		})
	}
}
