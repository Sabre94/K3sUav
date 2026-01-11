.PHONY: build build-image deploy clean test

# 变量
IMAGE_NAME := uav-agent
IMAGE_TAG := v0.3.0
DOCKER_REGISTRY := x1224403599
FULL_IMAGE := $(IMAGE_NAME):$(IMAGE_TAG)
REGISTRY_IMAGE := $(DOCKER_REGISTRY)/$(IMAGE_NAME):$(IMAGE_TAG)

SCHEDULER_IMAGE := uav-scheduler
SCHEDULER_TAG := v0.1.0
SCHEDULER_FULL_IMAGE := $(SCHEDULER_IMAGE):$(SCHEDULER_TAG)

TASK_CONTROLLER_IMAGE := uav-task-controller
TASK_CONTROLLER_TAG := v0.1.0
TASK_CONTROLLER_FULL_IMAGE := $(TASK_CONTROLLER_IMAGE):$(TASK_CONTROLLER_TAG)
TASK_CONTROLLER_REGISTRY_IMAGE := $(DOCKER_REGISTRY)/$(TASK_CONTROLLER_IMAGE):$(TASK_CONTROLLER_TAG)

# 编译二进制文件
build:
	@echo "🔨 编译 UAV Agent..."
	@export PATH=$$PATH:/usr/local/go/bin && \
	go build -o bin/uav-agent ./cmd/agent/
	@echo "✅ 编译完成: bin/uav-agent"

# 编译调度器
build-scheduler:
	@echo "🔨 编译 UAV Scheduler..."
	@export PATH=$$PATH:/usr/local/go/bin && \
	go build -o bin/uav-scheduler ./cmd/scheduler/
	@echo "✅ 编译完成: bin/uav-scheduler"

# 构建 Docker 镜像（K3s 方式）
build-image: build
	@echo "🐳 构建 Docker 镜像..."
	@docker build -t $(FULL_IMAGE) .
	@echo "📦 导入镜像到 K3s..."
	@docker save $(FULL_IMAGE) | sudo k3s ctr images import -
	@echo "✅ 镜像已就绪: $(FULL_IMAGE)"

# 构建并推送到 Docker Hub
build-and-push: build
	@echo "🐳 构建 Docker 镜像 (多架构)..."
	@docker build -t $(FULL_IMAGE) -t $(REGISTRY_IMAGE) .
	@echo "📤 推送镜像到 Docker Hub..."
	@docker push $(REGISTRY_IMAGE)
	@echo "✅ 镜像已推送: $(REGISTRY_IMAGE)"

# 推送镜像到 Docker Hub（需要先 build-image）
push:
	@echo "🏷️  标记镜像..."
	@docker tag $(FULL_IMAGE) $(REGISTRY_IMAGE)
	@echo "📤 推送镜像到 Docker Hub..."
	@docker push $(REGISTRY_IMAGE)
	@echo "✅ 镜像已推送: $(REGISTRY_IMAGE)"

# 快速构建（跳过 Docker，直接编译用于测试）
build-quick:
	@echo "⚡ 快速编译..."
	@export PATH=$$PATH:/usr/local/go/bin && \
	CGO_ENABLED=0 go build -ldflags="-s -w" -o bin/uav-agent ./cmd/agent/
	@echo "✅ 完成"

# 部署到 K3s
deploy:
	@./deploy/deploy.sh

# 仅部署 CRD
deploy-crd:
	@echo "📋 部署 CRD..."
	@kubectl apply -f api/crd/uav-metrics-crd.yaml
	@echo "✅ CRD 已部署"

# 部署 DaemonSet
deploy-daemonset:
	@echo "🚀 部署 DaemonSet..."
	@kubectl apply -f deploy/agent-daemonset.yaml
	@echo "✅ DaemonSet 已部署"

# 查看状态
status:
	@echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
	@echo "  Pod 状态"
	@echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
	@kubectl get pods -l app=uav-agent -o wide
	@echo ""
	@echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
	@echo "  UAVMetrics 资源"
	@echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
	@kubectl get uavmetrics -A

# 查看日志
logs:
	@kubectl logs -l app=uav-agent -f --max-log-requests=10

# 查看特定节点的日志
logs-node:
	@read -p "输入节点名称: " node && \
	POD=$$(kubectl get pods -l app=uav-agent --field-selector spec.nodeName=$$node -o jsonpath='{.items[0].metadata.name}') && \
	kubectl logs $$POD -f

# 测试运行（本地）
test-local:
	@echo "🧪 本地测试运行..."
	@export NODE_NAME=$$(hostname) && \
	export KUBECONFIG=/etc/rancher/k3s/k3s.yaml && \
	export LOG_LEVEL=debug && \
	./bin/uav-agent

# 清理
clean:
	@echo "🗑️  清理资源..."
	@kubectl delete -f deploy/agent-daemonset.yaml || true
	@rm -f bin/uav-agent
	@echo "✅ 清理完成"

# 完全清理（包括 CRD）
clean-all: clean
	@kubectl delete -f api/crd/uav-metrics-crd.yaml || true
	@echo "✅ 所有资源已清理"

# 构建调度器镜像
build-scheduler-image: build-scheduler
	@echo "🐳 构建 Scheduler Docker 镜像..."
	@docker build -f Dockerfile.scheduler -t $(SCHEDULER_FULL_IMAGE) .
	@echo "📦 导入镜像到 K3s..."
	@docker save $(SCHEDULER_FULL_IMAGE) | sudo k3s ctr images import -
	@echo "✅ 镜像已就绪: $(SCHEDULER_FULL_IMAGE)"

# 部署调度器
deploy-scheduler:
	@echo "🚀 部署 Scheduler..."
	@kubectl apply -f deploy/scheduler-deployment.yaml
	@echo "✅ Scheduler 已部署"

# 查看调度器状态
scheduler-status:
	@echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
	@echo "  Scheduler Pod 状态"
	@echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
	@kubectl get pods -l app=uav-scheduler -o wide

# 查看调度器日志
scheduler-logs:
	@kubectl logs -l app=uav-scheduler -f

# 测试调度器（本地）
test-scheduler:
	@echo "🧪 本地测试调度器..."
	@export KUBECONFIG=/etc/rancher/k3s/k3s.yaml && \
	export LOG_LEVEL=debug && \
	export ALGORITHM_NAME=distance-based && \
	./bin/uav-scheduler

# 清理调度器
clean-scheduler:
	@echo "🗑️  清理 Scheduler..."
	@kubectl delete -f deploy/scheduler-deployment.yaml || true
	@rm -f bin/uav-scheduler
	@echo "✅ Scheduler 清理完成"

# 查看帮助
help:
	@echo "UAV Project Makefile 命令:"
	@echo ""
	@echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
	@echo "  UAV Agent 命令"
	@echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
	@echo "  make build          - 编译 Agent 二进制文件"
	@echo "  make build-image    - 构建 Agent Docker 镜像"
	@echo "  make deploy         - 完整部署（CRD + DaemonSet）"
	@echo "  make deploy-crd     - 仅部署 CRD"
	@echo "  make deploy-daemonset - 仅部署 DaemonSet"
	@echo "  make status         - 查看 Agent 部署状态"
	@echo "  make logs           - 查看所有 Agent 日志"
	@echo "  make test-local     - 本地测试 Agent"
	@echo "  make clean          - 清理 Agent"
	@echo "  make clean-all      - 完全清理（包括 CRD）"
	@echo ""
	@echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
	@echo "  UAV Scheduler 命令"
	@echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
	@echo "  make build-scheduler       - 编译 Scheduler 二进制"
	@echo "  make build-scheduler-image - 构建 Scheduler 镜像"
	@echo "  make deploy-scheduler      - 部署 Scheduler"
	@echo "  make scheduler-status      - 查看 Scheduler 状态"
	@echo "  make scheduler-logs        - 查看 Scheduler 日志"
	@echo "  make test-scheduler        - 本地测试 Scheduler"
	@echo "  make clean-scheduler       - 清理 Scheduler"
	@echo ""

# ============================================
# Task Controller (自动调度控制器)
# ============================================

# 编译 Task Controller
build-task-controller:
	@echo "🔨 编译 UAV Task Controller..."
	@export PATH=$$PATH:/usr/local/go/bin && \
	go build -o bin/uav-task-controller ./cmd/task-controller/
	@echo "✅ 编译完成: bin/uav-task-controller"

# 构建 Task Controller 镜像
build-task-controller-image: build-task-controller
	@echo "🐳 构建 Task Controller Docker 镜像..."
	@docker build -f Dockerfile.task-controller -t $(TASK_CONTROLLER_FULL_IMAGE) -t $(TASK_CONTROLLER_REGISTRY_IMAGE) .
	@echo "✅ 镜像已构建: $(TASK_CONTROLLER_FULL_IMAGE)"

# 推送 Task Controller 镜像
push-task-controller:
	@echo "📤 推送 Task Controller 镜像到 Docker Hub..."
	@docker push $(TASK_CONTROLLER_REGISTRY_IMAGE)
	@echo "✅ 镜像已推送: $(TASK_CONTROLLER_REGISTRY_IMAGE)"

# 部署 UAVTask CRD
deploy-task-crd:
	@echo "📋 部署 UAVTask CRD..."
	@kubectl apply -f api/crd/uav-task-crd.yaml
	@echo "✅ UAVTask CRD 已部署"

# 部署 Task Controller
deploy-task-controller:
	@echo "🚀 部署 Task Controller..."
	@kubectl apply -f deploy/task-controller-deployment.yaml
	@echo "✅ Task Controller 已部署"

# 查看 Task Controller 状态
task-controller-status:
	@echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
	@echo "  Task Controller Pod 状态"
	@echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
	@kubectl get pods -l app=uav-task-controller -o wide

# 查看 Task Controller 日志
task-controller-logs:
	@kubectl logs -l app=uav-task-controller -f

# 查看所有 UAVTask 资源
list-tasks:
	@echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
	@echo "  UAVTask 资源列表"
	@echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
	@kubectl get uavtasks -A

# 创建示例任务
create-example-task:
	@echo "🚀 创建示例 UAVTask..."
	@kubectl apply -f examples/uavtask-example.yaml
	@echo "✅ 示例任务已创建"

# 清理 Task Controller
clean-task-controller:
	@echo "🗑️  清理 Task Controller..."
	@kubectl delete -f deploy/task-controller-deployment.yaml || true
	@echo "✅ Task Controller 已清理"

# 完整部署（CRD + Controller）
deploy-task-system: deploy-task-crd deploy-task-controller
	@echo "✅ UAV Task 系统部署完成"
