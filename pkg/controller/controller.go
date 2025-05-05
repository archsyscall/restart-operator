package controller

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/archsyscall/restart-operator/pkg/apis/v1alpha1"
	"github.com/go-logr/logr"
	"github.com/robfig/cron/v3"
	appsv1 "k8s.io/api/apps/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

type RestartScheduleReconciler struct {
	client.Client
	Scheme   *runtime.Scheme
	Recorder record.EventRecorder
	Log      logr.Logger

	cron        *cron.Cron
	scheduleIDs map[string]cron.EntryID
	mu          sync.RWMutex
}

func NewRestartScheduleReconciler(
	client client.Client,
	scheme *runtime.Scheme,
	recorder record.EventRecorder,
) *RestartScheduleReconciler {
	cronScheduler := cron.New()
	cronScheduler.Start()

	return &RestartScheduleReconciler{
		Client:      client,
		Scheme:      scheme,
		Recorder:    recorder,
		Log:         log.Log.WithName("controller").WithName("RestartSchedule"),
		cron:        cronScheduler,
		scheduleIDs: make(map[string]cron.EntryID),
		mu:          sync.RWMutex{},
	}
}

// +kubebuilder:rbac:groups=restart-operator.k8s,resources=restartschedules,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=restart-operator.k8s,resources=restartschedules/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=restart-operator.k8s,resources=restartschedules/finalizers,verbs=update
// +kubebuilder:rbac:groups=apps,resources=deployments,verbs=get;list;watch;update;patch
// +kubebuilder:rbac:groups=apps,resources=statefulsets,verbs=get;list;watch;update;patch
// +kubebuilder:rbac:groups=apps,resources=daemonsets,verbs=get;list;watch;update;patch
// +kubebuilder:rbac:groups="",resources=events,verbs=create;patch
func (r *RestartScheduleReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx).WithValues("restartschedule", req.NamespacedName)
	logger.Info("Reconciling RestartSchedule")

	var restartSchedule v1alpha1.RestartSchedule
	if err := r.Get(ctx, req.NamespacedName, &restartSchedule); err != nil {
		if errors.IsNotFound(err) {
			r.mu.Lock()
			defer r.mu.Unlock()

			if id, exists := r.scheduleIDs[req.String()]; exists {
				r.cron.Remove(id)
				delete(r.scheduleIDs, req.String())
				logger.Info("Removed schedule for deleted RestartSchedule")
			}
			return ctrl.Result{}, nil
		}
		logger.Error(err, "Failed to get RestartSchedule")
		return ctrl.Result{}, err
	}

	cronSchedule, err := cron.ParseStandard(restartSchedule.Spec.Schedule)
	if err != nil {
		logger.Error(err, "Invalid cron schedule", "schedule", restartSchedule.Spec.Schedule)

		condition := metav1.Condition{
			Type:               "Valid",
			Status:             metav1.ConditionFalse,
			Reason:             "InvalidSchedule",
			Message:            fmt.Sprintf("Invalid schedule: %v", err),
			LastTransitionTime: metav1.Now(),
		}
		applyCondition(&restartSchedule, condition)

		if updateErr := r.Status().Update(ctx, &restartSchedule); updateErr != nil {
			logger.Error(updateErr, "Failed to update RestartSchedule status with error condition")
		}

		r.Recorder.Event(&restartSchedule, "Warning", "InvalidSchedule", fmt.Sprintf("Invalid schedule: %v", err))

		return ctrl.Result{}, err
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	if id, exists := r.scheduleIDs[req.String()]; exists {
		r.cron.Remove(id)
		delete(r.scheduleIDs, req.String())
		logger.Info("Removed existing schedule", "id", id)
	}

	logger.Info("Adding new schedule", "schedule", restartSchedule.Spec.Schedule)

	id, err := r.cron.AddFunc(restartSchedule.Spec.Schedule, func() {
		jobCtx := context.Background()
		jobLogger := r.Log.WithValues(
			"restartschedule", req.NamespacedName,
			"execution", time.Now().Format(time.RFC3339),
		)

		var latestSchedule v1alpha1.RestartSchedule
		err := r.Get(jobCtx, req.NamespacedName, &latestSchedule)
		if err != nil {
			jobLogger.Error(err, "Failed to get latest RestartSchedule for scheduled restart")
			return
		}

		if err := r.restartResource(jobCtx, &latestSchedule); err != nil {
			jobLogger.Error(err, "Failed to restart resource")
			return
		}

		now := metav1.Now()
		latestSchedule.Status.LastSuccessfulTime = &now

		next := cronSchedule.Next(time.Now())
		latestSchedule.Status.NextScheduledTime = &metav1.Time{Time: next}

		if err := r.Status().Update(jobCtx, &latestSchedule); err != nil {
			jobLogger.Error(err, "Failed to update status after restart")
		}
	})

	if err != nil {
		logger.Error(err, "Failed to add cron job")
		return ctrl.Result{}, err
	}

	r.scheduleIDs[req.String()] = id

	next := cronSchedule.Next(time.Now())
	restartSchedule.Status.NextScheduledTime = &metav1.Time{Time: next}

	condition := metav1.Condition{
		Type:               "Valid",
		Status:             metav1.ConditionTrue,
		Reason:             "ScheduleValid",
		Message:            "Schedule is valid and has been registered",
		LastTransitionTime: metav1.Now(),
	}
	applyCondition(&restartSchedule, condition)

	if err := r.Status().Update(ctx, &restartSchedule); err != nil {
		logger.Error(err, "Failed to update RestartSchedule status")
		return ctrl.Result{}, err
	}

	logger.Info("Successfully reconciled RestartSchedule",
		"nextRun", next.Format(time.RFC3339))

	return ctrl.Result{}, nil
}

func (r *RestartScheduleReconciler) restartResource(ctx context.Context, schedule *v1alpha1.RestartSchedule) error {
	logger := r.Log.WithValues(
		"restartschedule", types.NamespacedName{Name: schedule.Name, Namespace: schedule.Namespace},
		"targetKind", schedule.Spec.TargetRef.Kind,
		"targetName", schedule.Spec.TargetRef.Name,
	)

	targetNamespace := schedule.Spec.TargetRef.Namespace
	if targetNamespace == "" {
		targetNamespace = schedule.Namespace
		logger.Info("Using RestartSchedule namespace for target", "namespace", targetNamespace)
	}

	switch schedule.Spec.TargetRef.Kind {
	case "Deployment":
		return r.restartDeployment(ctx, schedule.Spec.TargetRef.Name, targetNamespace)
	case "StatefulSet":
		return r.restartStatefulSet(ctx, schedule.Spec.TargetRef.Name, targetNamespace)
	case "DaemonSet":
		return r.restartDaemonSet(ctx, schedule.Spec.TargetRef.Name, targetNamespace)
	default:
		err := fmt.Errorf("unsupported resource kind: %s", schedule.Spec.TargetRef.Kind)
		logger.Error(err, "Unsupported target kind")
		r.Recorder.Event(schedule, "Warning", "UnsupportedKind",
			fmt.Sprintf("Unsupported target kind: %s", schedule.Spec.TargetRef.Kind))
		return err
	}
}

func (r *RestartScheduleReconciler) restartDeployment(ctx context.Context, name, namespace string) error {
	logger := r.Log.WithValues("kind", "Deployment", "name", name, "namespace", namespace)
	logger.Info("Restarting Deployment")

	var deployment appsv1.Deployment
	if err := r.Get(ctx, types.NamespacedName{Name: name, Namespace: namespace}, &deployment); err != nil {
		logger.Error(err, "Failed to get Deployment")
		return err
	}

	if deployment.Spec.Template.ObjectMeta.Annotations == nil {
		deployment.Spec.Template.ObjectMeta.Annotations = make(map[string]string)
	}

	deployment.Spec.Template.ObjectMeta.Annotations["restart-operator.k8s/restartedAt"] = time.Now().Format(time.RFC3339)

	if err := r.Update(ctx, &deployment); err != nil {
		logger.Error(err, "Failed to update Deployment")
		return err
	}

	logger.Info("Successfully restarted Deployment")
	return nil
}

func (r *RestartScheduleReconciler) restartStatefulSet(ctx context.Context, name, namespace string) error {
	logger := r.Log.WithValues("kind", "StatefulSet", "name", name, "namespace", namespace)
	logger.Info("Restarting StatefulSet")

	var statefulSet appsv1.StatefulSet
	if err := r.Get(ctx, types.NamespacedName{Name: name, Namespace: namespace}, &statefulSet); err != nil {
		logger.Error(err, "Failed to get StatefulSet")
		return err
	}

	if statefulSet.Spec.Template.ObjectMeta.Annotations == nil {
		statefulSet.Spec.Template.ObjectMeta.Annotations = make(map[string]string)
	}

	statefulSet.Spec.Template.ObjectMeta.Annotations["restart-operator.k8s/restartedAt"] = time.Now().Format(time.RFC3339)

	if err := r.Update(ctx, &statefulSet); err != nil {
		logger.Error(err, "Failed to update StatefulSet")
		return err
	}

	logger.Info("Successfully restarted StatefulSet")
	return nil
}

func (r *RestartScheduleReconciler) restartDaemonSet(ctx context.Context, name, namespace string) error {
	logger := r.Log.WithValues("kind", "DaemonSet", "name", name, "namespace", namespace)
	logger.Info("Restarting DaemonSet")

	var daemonSet appsv1.DaemonSet
	if err := r.Get(ctx, types.NamespacedName{Name: name, Namespace: namespace}, &daemonSet); err != nil {
		logger.Error(err, "Failed to get DaemonSet")
		return err
	}

	if daemonSet.Spec.Template.ObjectMeta.Annotations == nil {
		daemonSet.Spec.Template.ObjectMeta.Annotations = make(map[string]string)
	}

	daemonSet.Spec.Template.ObjectMeta.Annotations["restart-operator.k8s/restartedAt"] = time.Now().Format(time.RFC3339)

	if err := r.Update(ctx, &daemonSet); err != nil {
		logger.Error(err, "Failed to update DaemonSet")
		return err
	}

	logger.Info("Successfully restarted DaemonSet")
	return nil
}

func applyCondition(schedule *v1alpha1.RestartSchedule, condition metav1.Condition) {
	currentConditions := schedule.Status.Conditions
	for i, existingCondition := range currentConditions {
		if existingCondition.Type == condition.Type {
			if existingCondition.Status == condition.Status &&
				existingCondition.Reason == condition.Reason &&
				existingCondition.Message == condition.Message {
				return
			}
			currentConditions[i] = condition
			schedule.Status.Conditions = currentConditions
			return
		}
	}

	schedule.Status.Conditions = append(schedule.Status.Conditions, condition)
}

func (r *RestartScheduleReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&v1alpha1.RestartSchedule{}).
		Complete(r)
}
