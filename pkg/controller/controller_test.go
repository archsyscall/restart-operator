package controller

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/archsyscall/restart-operator/pkg/apis/v1alpha1"
	"github.com/robfig/cron/v3"
	"github.com/stretchr/testify/assert"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
)

func init() {
	logf.SetLogger(zap.New(zap.UseDevMode(true)))
}

func TestCronScheduleValidation(t *testing.T) {
	s := runtime.NewScheme()
	_ = v1alpha1.AddToScheme(s)
	_ = appsv1.AddToScheme(s)

	validSchedule := "0 * * * *"
	invalidSchedule := "invalid cron"

	tests := []struct {
		name        string
		schedule    string
		shouldError bool
	}{
		{
			name:        "Valid schedule",
			schedule:    validSchedule,
			shouldError: false,
		},
		{
			name:        "Invalid schedule",
			schedule:    invalidSchedule,
			shouldError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := cron.ParseStandard(tt.schedule)
			if tt.shouldError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestTargetNamespaceResolution(t *testing.T) {
	s := runtime.NewScheme()
	_ = v1alpha1.AddToScheme(s)
	_ = appsv1.AddToScheme(s)

	mockClient := fake.NewClientBuilder().WithScheme(s).Build()
	recorder := record.NewFakeRecorder(10)
	log := logf.Log.WithName("test-logger")

	reconciler := &RestartScheduleReconciler{
		Client:      mockClient,
		Scheme:      s,
		Recorder:    recorder,
		Log:         log,
		cron:        cron.New(),
		scheduleIDs: make(map[string]cron.EntryID),
		mu:          sync.RWMutex{},
	}

	scheduleWithNamespace := &v1alpha1.RestartSchedule{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-schedule",
			Namespace: "default",
		},
		Spec: v1alpha1.RestartScheduleSpec{
			Schedule: "0 * * * *",
			TargetRef: v1alpha1.TargetRef{
				Kind:      "Deployment",
				Name:      "test-deployment",
				Namespace: "explicit-namespace",
			},
		},
	}

	scheduleWithoutNamespace := &v1alpha1.RestartSchedule{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-schedule",
			Namespace: "default",
		},
		Spec: v1alpha1.RestartScheduleSpec{
			Schedule: "0 * * * *",
			TargetRef: v1alpha1.TargetRef{
				Kind: "Deployment",
				Name: "test-deployment",
			},
		},
	}

	targetNs := reconciler.getTargetNamespace(scheduleWithNamespace)
	assert.Equal(t, "explicit-namespace", targetNs)

	targetNs = reconciler.getTargetNamespace(scheduleWithoutNamespace)
	assert.Equal(t, "default", targetNs)
}

func TestDeploymentRestart(t *testing.T) {
	s := runtime.NewScheme()
	_ = v1alpha1.AddToScheme(s)
	_ = appsv1.AddToScheme(s)

	deployment := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-deployment",
			Namespace: "default",
		},
		Spec: appsv1.DeploymentSpec{
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{"app": "test"},
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{"app": "test"},
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name:  "test-container",
							Image: "nginx:latest",
						},
					},
				},
			},
		},
	}

	mockClient := fake.NewClientBuilder().
		WithScheme(s).
		WithObjects(deployment).
		Build()

	recorder := record.NewFakeRecorder(10)
	log := logf.Log.WithName("test-logger")

	reconciler := &RestartScheduleReconciler{
		Client:      mockClient,
		Scheme:      s,
		Recorder:    recorder,
		Log:         log,
		cron:        cron.New(),
		scheduleIDs: make(map[string]cron.EntryID),
		mu:          sync.RWMutex{},
	}

	err := reconciler.restartDeployment(context.Background(), "test-deployment", "default")
	assert.NoError(t, err)

	updatedDeployment := &appsv1.Deployment{}
	err = mockClient.Get(context.Background(),
		types.NamespacedName{Name: "test-deployment", Namespace: "default"},
		updatedDeployment)
	assert.NoError(t, err)

	annotations := updatedDeployment.Spec.Template.ObjectMeta.Annotations
	assert.NotEmpty(t, annotations)

	assert.Contains(t, annotations, "restart-operator.k8s/restartedAt")

	timestamp := annotations["restart-operator.k8s/restartedAt"]
	_, err = time.Parse(time.RFC3339, timestamp)
	assert.NoError(t, err)
}

func TestReconcileScheduleLifecycle(t *testing.T) {
	s := runtime.NewScheme()
	_ = v1alpha1.AddToScheme(s)
	_ = appsv1.AddToScheme(s)

	schedule := &v1alpha1.RestartSchedule{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-schedule",
			Namespace: "default",
		},
		Spec: v1alpha1.RestartScheduleSpec{
			Schedule: "0 * * * *",
			TargetRef: v1alpha1.TargetRef{
				Kind:      "Deployment",
				Name:      "test-deployment",
				Namespace: "default",
			},
		},
	}

	mockClient := fake.NewClientBuilder().
		WithScheme(s).
		WithObjects(schedule).
		Build()

	recorder := record.NewFakeRecorder(10)
	log := logf.Log.WithName("test-logger")

	reconciler := &RestartScheduleReconciler{
		Client:      mockClient,
		Scheme:      s,
		Recorder:    recorder,
		Log:         log,
		cron:        cron.New(),
		scheduleIDs: make(map[string]cron.EntryID),
		mu:          sync.RWMutex{},
	}
	reconciler.cron.Start()
	defer reconciler.cron.Stop()

	req := ctrl.Request{
		NamespacedName: types.NamespacedName{
			Name:      "test-schedule",
			Namespace: "default",
		},
	}

	reconciler.Client = &fakeStatusClient{Client: mockClient}

	_, err := reconciler.Reconcile(context.Background(), req)
	assert.NoError(t, err)

	reconciler.mu.RLock()
	_, exists := reconciler.scheduleIDs[req.String()]
	reconciler.mu.RUnlock()
	assert.True(t, exists)

	err = mockClient.Delete(context.Background(), schedule)
	assert.NoError(t, err)

	_, err = reconciler.Reconcile(context.Background(), req)
	assert.NoError(t, err)

	reconciler.mu.RLock()
	_, exists = reconciler.scheduleIDs[req.String()]
	reconciler.mu.RUnlock()
	assert.False(t, exists)
}

func TestApplyCondition(t *testing.T) {
	schedule := &v1alpha1.RestartSchedule{}

	condition1 := metav1.Condition{
		Type:    "Test",
		Status:  metav1.ConditionTrue,
		Reason:  "TestReason",
		Message: "Test message",
	}

	applyCondition(schedule, condition1)
	assert.Len(t, schedule.Status.Conditions, 1)
	assert.Equal(t, "Test", schedule.Status.Conditions[0].Type)
	assert.Equal(t, metav1.ConditionTrue, schedule.Status.Conditions[0].Status)

	condition2 := metav1.Condition{
		Type:    "Test",
		Status:  metav1.ConditionFalse,
		Reason:  "AnotherReason",
		Message: "Another message",
	}

	applyCondition(schedule, condition2)
	assert.Len(t, schedule.Status.Conditions, 1)
	assert.Equal(t, metav1.ConditionFalse, schedule.Status.Conditions[0].Status)
	assert.Equal(t, "AnotherReason", schedule.Status.Conditions[0].Reason)

	condition3 := metav1.Condition{
		Type:    "AnotherTest",
		Status:  metav1.ConditionTrue,
		Reason:  "SomeReason",
		Message: "Some message",
	}

	applyCondition(schedule, condition3)
	assert.Len(t, schedule.Status.Conditions, 2)
}

func (r *RestartScheduleReconciler) getTargetNamespace(schedule *v1alpha1.RestartSchedule) string {
	targetNamespace := schedule.Spec.TargetRef.Namespace
	if targetNamespace == "" {
		targetNamespace = schedule.Namespace
	}
	return targetNamespace
}

type fakeStatusClient struct {
	client.Client
}

func (c *fakeStatusClient) Status() client.StatusWriter {
	return &fakeStatusWriter{Client: c.Client}
}

type fakeStatusWriter struct {
	client.Client
}

func (sw *fakeStatusWriter) Update(ctx context.Context, obj client.Object, opts ...client.SubResourceUpdateOption) error {
	return sw.Client.Update(ctx, obj)
}

func (sw *fakeStatusWriter) Patch(ctx context.Context, obj client.Object, patch client.Patch, opts ...client.SubResourcePatchOption) error {
	return sw.Client.Patch(ctx, obj, patch)
}

func (sw *fakeStatusWriter) Create(ctx context.Context, obj client.Object, subResource client.Object, opts ...client.SubResourceCreateOption) error {
	return nil
}
