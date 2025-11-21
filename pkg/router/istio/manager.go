package istio

import (
	"context"
	"fmt"

	"github.com/sirupsen/logrus"
	"istio.io/client-go/pkg/apis/networking/v1beta1"
	versionedclient "istio.io/client-go/pkg/clientset/versioned"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// Manager DestinationRule 管理器
// 负责 DestinationRule 的 CRUD 操作
type Manager struct {
	istioClient versionedclient.Interface
	log         *logrus.Logger
}

// NewManager 创建管理器
func NewManager(istioClient versionedclient.Interface, log *logrus.Logger) *Manager {
	return &Manager{
		istioClient: istioClient,
		log:         log,
	}
}

// Apply 应用 DestinationRule（创建或更新）
func (m *Manager) Apply(ctx context.Context, dr *v1beta1.DestinationRule) error {
	if dr == nil {
		return fmt.Errorf("destination rule is nil")
	}

	m.log.WithFields(logrus.Fields{
		"name":      dr.Name,
		"namespace": dr.Namespace,
		"host":      dr.Spec.Host,
	}).Debug("Applying DestinationRule")

	// 尝试获取现有的 DestinationRule
	existing, err := m.istioClient.NetworkingV1beta1().DestinationRules(dr.Namespace).Get(
		ctx,
		dr.Name,
		metav1.GetOptions{},
	)

	if errors.IsNotFound(err) {
		// 不存在，创建新的
		_, err = m.istioClient.NetworkingV1beta1().DestinationRules(dr.Namespace).Create(
			ctx,
			dr,
			metav1.CreateOptions{},
		)
		if err != nil {
			return fmt.Errorf("failed to create DestinationRule: %w", err)
		}

		m.log.WithFields(logrus.Fields{
			"name":      dr.Name,
			"namespace": dr.Namespace,
		}).Info("DestinationRule created")

		return nil
	}

	if err != nil {
		return fmt.Errorf("failed to get DestinationRule: %w", err)
	}

	// 已存在，更新
	dr.ResourceVersion = existing.ResourceVersion // 必须设置 ResourceVersion
	_, err = m.istioClient.NetworkingV1beta1().DestinationRules(dr.Namespace).Update(
		ctx,
		dr,
		metav1.UpdateOptions{},
	)
	if err != nil {
		return fmt.Errorf("failed to update DestinationRule: %w", err)
	}

	m.log.WithFields(logrus.Fields{
		"name":      dr.Name,
		"namespace": dr.Namespace,
	}).Info("DestinationRule updated")

	return nil
}

// Delete 删除 DestinationRule
func (m *Manager) Delete(ctx context.Context, name, namespace string) error {
	m.log.WithFields(logrus.Fields{
		"name":      name,
		"namespace": namespace,
	}).Debug("Deleting DestinationRule")

	err := m.istioClient.NetworkingV1beta1().DestinationRules(namespace).Delete(
		ctx,
		name,
		metav1.DeleteOptions{},
	)

	if errors.IsNotFound(err) {
		// 已经不存在，视为成功
		m.log.WithFields(logrus.Fields{
			"name":      name,
			"namespace": namespace,
		}).Debug("DestinationRule not found (already deleted)")
		return nil
	}

	if err != nil {
		return fmt.Errorf("failed to delete DestinationRule: %w", err)
	}

	m.log.WithFields(logrus.Fields{
		"name":      name,
		"namespace": namespace,
	}).Info("DestinationRule deleted")

	return nil
}

// Get 获取 DestinationRule
func (m *Manager) Get(ctx context.Context, name, namespace string) (*v1beta1.DestinationRule, error) {
	dr, err := m.istioClient.NetworkingV1beta1().DestinationRules(namespace).Get(
		ctx,
		name,
		metav1.GetOptions{},
	)
	if err != nil {
		return nil, err
	}
	return dr, nil
}

// List 列出所有由 UAV Router 管理的 DestinationRules
func (m *Manager) List(ctx context.Context, namespace string) ([]*v1beta1.DestinationRule, error) {
	drList, err := m.istioClient.NetworkingV1beta1().DestinationRules(namespace).List(
		ctx,
		metav1.ListOptions{
			LabelSelector: "app.kubernetes.io/managed-by=uav-router",
		},
	)
	if err != nil {
		return nil, err
	}

	result := make([]*v1beta1.DestinationRule, 0, len(drList.Items))
	for i := range drList.Items {
		result = append(result, drList.Items[i])
	}

	return result, nil
}

// Cleanup 清理所有由 UAV Router 管理的 DestinationRules
func (m *Manager) Cleanup(ctx context.Context, namespace string) error {
	m.log.WithField("namespace", namespace).Info("Cleaning up all UAV-managed DestinationRules")

	err := m.istioClient.NetworkingV1beta1().DestinationRules(namespace).DeleteCollection(
		ctx,
		metav1.DeleteOptions{},
		metav1.ListOptions{
			LabelSelector: "app.kubernetes.io/managed-by=uav-router",
		},
	)

	if err != nil {
		return fmt.Errorf("failed to cleanup DestinationRules: %w", err)
	}

	m.log.WithField("namespace", namespace).Info("Cleanup completed")
	return nil
}
