package scheduler

import (
	"context"
	"fmt"
	"sort"
	"time"

	"github.com/k3suav/uav-monitor/pkg/k8s"
	"github.com/k3suav/uav-monitor/pkg/scheduler/algorithm"
	"github.com/k3suav/uav-monitor/pkg/scheduler/algorithm/greed_nsgaii"
	"github.com/k3suav/uav-monitor/pkg/scheduler/config"
	"github.com/sirupsen/logrus"

	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
)

// Scheduler UAV 自定义调度器
type Scheduler struct {
	config        *config.SchedulerConfig
	k8sClientset  *kubernetes.Clientset
	uavClient     *k8s.Client
	algorithm     algorithm.SchedulingAlgorithm // 默认算法
	algoFactory   *algorithm.AlgorithmFactory   // 算法工厂（用于 Pod 级别算法）
	log           *logrus.Logger

	// 自适应调度集成（仅用于覆盖率算法）
	adaptiveIntegration *AdaptiveSchedulerIntegration
}

// NewScheduler 创建新的调度器
func NewScheduler(cfg *config.SchedulerConfig, algo algorithm.SchedulingAlgorithm, uavClient *k8s.Client) (*Scheduler, error) {
	// 创建 K8s clientset
	var k8sConfig *rest.Config
	var err error

	if cfg.KubeconfigPath == "" {
		k8sConfig, err = rest.InClusterConfig()
		if err != nil {
			kubeconfigPath := clientcmd.RecommendedHomeFile
			k8sConfig, err = clientcmd.BuildConfigFromFlags("", kubeconfigPath)
			if err != nil {
				return nil, fmt.Errorf("failed to build kubernetes config: %w", err)
			}
		}
	} else {
		k8sConfig, err = clientcmd.BuildConfigFromFlags("", cfg.KubeconfigPath)
		if err != nil {
			return nil, fmt.Errorf("failed to build kubernetes config: %w", err)
		}
	}

	clientset, err := kubernetes.NewForConfig(k8sConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to create kubernetes clientset: %w", err)
	}

	// 初始化日志
	log := logrus.New()
	log.SetFormatter(&logrus.TextFormatter{
		FullTimestamp:   true,
		TimestampFormat: "2006-01-02 15:04:05",
	})

	switch cfg.LogLevel {
	case "debug":
		log.SetLevel(logrus.DebugLevel)
	case "info":
		log.SetLevel(logrus.InfoLevel)
	case "warn":
		log.SetLevel(logrus.WarnLevel)
	case "error":
		log.SetLevel(logrus.ErrorLevel)
	default:
		log.SetLevel(logrus.InfoLevel)
	}

	if cfg.StructuredLogging {
		log.SetFormatter(&logrus.JSONFormatter{
			TimestampFormat: time.RFC3339,
		})
	}

	return &Scheduler{
		config:       cfg,
		k8sClientset: clientset,
		uavClient:    uavClient,
		algorithm:    algo,
		algoFactory:  algorithm.NewAlgorithmFactory(), // 初始化算法工厂
		log:          log,
	}, nil
}

// SetAdaptiveIntegration 设置自适应调度集成（仅用于覆盖率算法）
func (s *Scheduler) SetAdaptiveIntegration(integration *AdaptiveSchedulerIntegration) {
	s.adaptiveIntegration = integration
	s.log.Info("Adaptive scheduler integration configured (for coverage algorithms only)")
}

// Run 启动调度器
func (s *Scheduler) Run(ctx context.Context) error {
	s.log.WithFields(logrus.Fields{
		"schedulerName": s.config.SchedulerName,
		"algorithm":     s.algorithm.Name(),
	}).Info("Starting UAV Scheduler")

	// 启动 Pod watcher
	for {
		select {
		case <-ctx.Done():
			s.log.Info("Scheduler stopped")
			return ctx.Err()
		default:
			if err := s.watchAndSchedule(ctx); err != nil {
				s.log.WithError(err).Error("Watch and schedule error")
				time.Sleep(5 * time.Second) // 错误后等待重试
			}
		}
	}
}

// watchAndSchedule 监听并调度 Pod
func (s *Scheduler) watchAndSchedule(ctx context.Context) error {
	// 创建 watcher 监听未调度的 Pod
	watcher, err := s.k8sClientset.CoreV1().Pods(s.config.Namespace).Watch(ctx, metav1.ListOptions{
		FieldSelector: "spec.nodeName=", // 未分配节点
		// 注意：不能通过 label selector 过滤 schedulerName，需要在事件处理中检查
	})
	if err != nil {
		return fmt.Errorf("failed to watch pods: %w", err)
	}
	defer watcher.Stop()

	s.log.Info("Watching for unscheduled pods...")

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case event, ok := <-watcher.ResultChan():
			if !ok {
				return fmt.Errorf("watch channel closed")
			}

			if event.Type == watch.Added || event.Type == watch.Modified {
				pod, ok := event.Object.(*v1.Pod)
				if !ok {
					continue
				}

				// 检查是否是我们负责调度的 Pod
				if pod.Spec.SchedulerName != s.config.SchedulerName {
					continue
				}

				// 检查是否已经分配节点
				if pod.Spec.NodeName != "" {
					continue
				}

				// 执行调度
				s.log.WithFields(logrus.Fields{
					"pod":       pod.Name,
					"namespace": pod.Namespace,
				}).Info("Scheduling pod...")

				if err := s.schedulePod(ctx, pod); err != nil {
					s.log.WithError(err).WithField("pod", pod.Name).Error("Failed to schedule pod")
				}
			}
		}
	}
}

// schedulePod 调度单个 Pod
func (s *Scheduler) schedulePod(ctx context.Context, pod *v1.Pod) error {
	startTime := time.Now()

	// 1. 获取所有节点的 UAVMetrics
	metrics, err := s.uavClient.ListUAVMetrics(ctx)
	if err != nil {
		return fmt.Errorf("failed to list UAVMetrics: %w", err)
	}

	if len(metrics) == 0 {
		return fmt.Errorf("no UAV nodes available")
	}

	s.log.WithField("nodeCount", len(metrics)).Debug("Fetched UAVMetrics")

	// 1.5 【新增】根据 Pod annotation 选择算法
	selectedAlgo, err := s.algoFactory.CreateFromPod(pod, s.algorithm)
	if err != nil {
		s.log.WithError(err).Warn("Failed to create algorithm from pod annotation, using default")
		selectedAlgo = s.algorithm
	}

	// 记录使用的算法
	s.log.WithFields(logrus.Fields{
		"pod":       pod.Name,
		"algorithm": selectedAlgo.Name(),
	}).Debug("Algorithm selected for pod")

	// 2. 【coverage-based 和 greed-nsgaii 特殊处理】加锁确保串行调度（贪心算法）
	var coverageAlgo *algorithm.CoverageBasedAlgorithm
	var greedAlgo *greed_nsgaii.GreedNSGAIIAlgorithm

	if selectedAlgo.Name() == "coverage-based" {
		if ca, ok := selectedAlgo.(*algorithm.CoverageBasedAlgorithm); ok {
			coverageAlgo = ca
			// 加锁：确保同一个 Deployment 的多个 Pod 串行调度
			coverageAlgo.LockDeployment(getDeploymentName(pod))
			defer coverageAlgo.UnlockDeployment(getDeploymentName(pod))
		}
	} else if selectedAlgo.Name() == "greed-nsgaii" {
		// 获取底层的 GreedNSGAIIAlgorithm
		if adapter, ok := selectedAlgo.(*algorithm.GreedNSGAIIAdapter); ok {
			greedAlgo = adapter.GetUnderlyingAlgorithm()
		}
	}

	// 3. 过滤节点
	filteredMetrics := metrics
	if selectedAlgo.Filter != nil {
		filteredMetrics, err = selectedAlgo.Filter(ctx, pod, metrics)
		if err != nil {
			return fmt.Errorf("filter error: %w", err)
		}
		if len(filteredMetrics) == 0 {
			return fmt.Errorf("no nodes passed filter")
		}
		s.log.WithField("filteredCount", len(filteredMetrics)).Debug("Nodes filtered")
	}

	// 4. 计算分数
	scores, err := selectedAlgo.Score(ctx, pod, filteredMetrics)
	if err != nil {
		return fmt.Errorf("score error: %w", err)
	}

	if len(scores) == 0 {
		return fmt.Errorf("no scores returned")
	}

	// 5. 排序并选择最佳节点
	sort.Slice(scores, func(i, j int) bool {
		return scores[i].Score > scores[j].Score
	})

	bestNode := scores[0].NodeName
	bestScore := scores[0].Score

	// 记录前3名节点的分数（用于调试）
	topScores := scores
	if len(topScores) > 3 {
		topScores = topScores[:3]
	}

	s.log.WithFields(logrus.Fields{
		"pod":       pod.Name,
		"algorithm": selectedAlgo.Name(),
		"topScores": topScores,
	}).Debug("Scoring completed")

	// 6. 绑定 Pod 到节点
	if err := s.bindPodToNode(ctx, pod, bestNode); err != nil {
		return fmt.Errorf("bind error: %w", err)
	}

	// 7. 【贪心算法关键】绑定成功后，记录到算法的缓存
	if coverageAlgo != nil {
		// 增量覆盖 = 分数 / 100
		incrementalCoverage := bestScore / 100.0
		coverageAlgo.RecordBinding(pod, bestNode, incrementalCoverage)

		s.log.WithFields(logrus.Fields{
			"pod":  pod.Name,
			"node": bestNode,
			"incrementalCoverage": fmt.Sprintf("%.2f%%", incrementalCoverage),
		}).Debug("Recorded coverage binding")
	} else if greedAlgo != nil {
		// 记录 greed-nsgaii 算法的节点绑定
		greedAlgo.RecordBinding(pod, bestNode, metrics)

		s.log.WithFields(logrus.Fields{
			"pod":  pod.Name,
			"node": bestNode,
		}).Debug("Recorded greed-nsgaii binding")
	}

	// 8. 【自适应调度】只为覆盖率算法注册监控
	if s.adaptiveIntegration != nil && NeedsCoverageMonitor(selectedAlgo.Name()) {
		deploymentName := getDeploymentName(pod)

		// 检查是否已注册，如果没有则注册
		state := s.adaptiveIntegration.GetDeploymentState(deploymentName)
		if state == nil {
			// 首次调度，注册 Deployment
			if err := s.adaptiveIntegration.RegisterDeployment(deploymentName, pod.Namespace, []string{bestNode}); err != nil {
				s.log.WithError(err).Warn("Failed to register deployment for coverage monitoring")
			} else {
				s.log.WithFields(logrus.Fields{
					"deployment": deploymentName,
					"algorithm":  selectedAlgo.Name(),
				}).Info("Deployment registered for coverage monitoring")
			}
		}

		// 记录 Pod 绑定
		s.adaptiveIntegration.RecordPodBinding(deploymentName, pod.Name, bestNode)
	}

	duration := time.Since(startTime)

	s.log.WithFields(logrus.Fields{
		"pod":       pod.Name,
		"namespace": pod.Namespace,
		"node":      bestNode,
		"score":     fmt.Sprintf("%.2f", bestScore),
		"reason":    scores[0].Reason,
		"duration":  duration.Milliseconds(),
		"algorithm": selectedAlgo.Name(),
	}).Info("Pod scheduled successfully")

	return nil
}

// getDeploymentName 从 Pod 的 Owner References 获取 Deployment 名称
func getDeploymentName(pod *v1.Pod) string {
	for _, owner := range pod.OwnerReferences {
		if owner.Kind == "ReplicaSet" {
			return owner.Name
		}
	}
	return pod.Name
}

// bindPodToNode 绑定 Pod 到节点
func (s *Scheduler) bindPodToNode(ctx context.Context, pod *v1.Pod, nodeName string) error {
	binding := &v1.Binding{
		ObjectMeta: metav1.ObjectMeta{
			Name:      pod.Name,
			Namespace: pod.Namespace,
		},
		Target: v1.ObjectReference{
			Kind: "Node",
			Name: nodeName,
		},
	}

	err := s.k8sClientset.CoreV1().Pods(pod.Namespace).Bind(ctx, binding, metav1.CreateOptions{})
	if err != nil {
		return fmt.Errorf("failed to bind pod %s to node %s: %w", pod.Name, nodeName, err)
	}

	return nil
}
