/*
Copyright 2022 cnych.

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

package controllers

import (
	"context"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/util/json"
	"opdemo/resources"
	"reflect"

	"github.com/go-logr/logr"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	appv1beta1 "opdemo/api/v1beta1"
)

// AppServiceReconciler reconciles a AppService object
type AppServiceReconciler struct {
	client.Client
	Log    logr.Logger
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=app.ydzs.io,resources=appservices,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=app.ydzs.io,resources=appservices/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=app.ydzs.io,resources=appservices,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=apps,resources=deployments,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups="",resources=services,verbs=get;list;watch;create;update;patch;delete

func (r *AppServiceReconciler) Reconcile(req ctrl.Request) (ctrl.Result, error) {
	ctx := context.Background()
	log := r.Log.WithValues("appservice", req.NamespacedName)

	// 业务逻辑实现
	// 获取 AppService 实例
	var appService appv1beta1.AppService
	err := r.Get(ctx, req.NamespacedName, &appService)
	if err != nil {
		// MyApp 被删除的时候，忽略
		if client.IgnoreNotFound(err) != nil {
			return ctrl.Result{}, err
		}
		return ctrl.Result{}, nil
	}

	log.Info("fetch appservice objects", "appservice", appService)

	// 如果不存在，则创建关联资源
	// 如果存在，判断是否需要更新
	//   如果需要更新，则直接更新
	//   如果不需要更新，则正常返回
	oldSpecAnnotation := "spec"
	deploy := &appsv1.Deployment{}
	if err := r.Get(ctx, req.NamespacedName, deploy); err != nil && errors.IsNotFound(err) {
		// 1. 关联 Annotations
		data, _ := json.Marshal(appService.Spec)
		if appService.Annotations != nil {
			appService.Annotations[oldSpecAnnotation] = string(data)
		} else {
			appService.Annotations = map[string]string{oldSpecAnnotation: string(data)}
		}
		if err := r.Client.Update(ctx, &appService); err != nil {
			return ctrl.Result{}, err
		}
		// 创建关联资源
		// 2. 创建 Deployment
		deploy := resources.NewDeploy(&appService)
		if err := r.Client.Create(ctx, deploy); err != nil {
			return ctrl.Result{}, err
		}
		// 3. 创建 Service
		service := resources.NewService(&appService)
		if err := r.Create(ctx, service); err != nil {
			return ctrl.Result{}, err
		}
		return ctrl.Result{}, nil
	}
	oldspec := appv1beta1.AppServiceSpec{}
	if err := json.Unmarshal([]byte(appService.Annotations[oldSpecAnnotation]), &oldspec); err != nil {
		return ctrl.Result{}, err
	}
	// 当前规范与旧的对象不一致，则需要更新
	if !reflect.DeepEqual(appService.Spec, oldspec) {
		// 更新关联资源
		newDeploy := resources.NewDeploy(&appService)
		oldDeploy := &appsv1.Deployment{}
		if err := r.Get(ctx, req.NamespacedName, oldDeploy); err != nil {
			return ctrl.Result{}, err
		}
		oldDeploy.Spec = newDeploy.Spec
		if err := r.Client.Update(ctx, oldDeploy); err != nil {
			return ctrl.Result{}, err
		}

		newService := resources.NewService(&appService)
		oldService := &corev1.Service{}
		if err := r.Get(ctx, req.NamespacedName, oldService); err != nil {
			return ctrl.Result{}, err
		}
		// 需要指定 ClusterIP 为之前的，不然更新会报错
		newService.Spec.ClusterIP = oldService.Spec.ClusterIP
		oldService.Spec = newService.Spec
		if err := r.Client.Update(ctx, oldService); err != nil {
			return ctrl.Result{}, err
		}
		return ctrl.Result{}, nil
	}
	return ctrl.Result{}, nil
}

func (r *AppServiceReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&appv1beta1.AppService{}).
		Complete(r)
}
