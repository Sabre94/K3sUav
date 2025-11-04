.PHONY: build build-image deploy clean test

# 变量
IMAGE_NAME := uav-agent
IMAGE_TAG := v0.1.0
FULL_IMAGE := $(IMAGE_NAME):$(IMAGE_TAG)

# 编译二进制文件
build:
	@echo "🔨 编译 UAV Agent..."
	@export PATH=$$PATH:/usr/local/go/bin && \
	go build -o bin/uav-agent ./cmd/agent/
	@echo "✅ 编译完成: bin/uav-agent"

# 构建 Docker 镜像（K3s 方式）
build-image: build
	@echo "🐳 构建 Docker 镜像..."
	@docker build -t $(FULL_IMAGE) .
	@echo "📦 导入镜像到 K3s..."
	@docker save $(FULL_IMAGE) | sudo k3s ctr images import -
	@echo "✅ 镜像已就绪: $(FULL_IMAGE)"

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

# 查看帮助
help:
	@echo "UAV Agent Makefile 命令:"
	@echo ""
	@echo "  make build          - 编译二进制文件"
	@echo "  make build-image    - 构建 Docker 镜像"
	@echo "  make deploy         - 完整部署（CRD + DaemonSet）"
	@echo "  make deploy-crd     - 仅部署 CRD"
	@echo "  make deploy-daemonset - 仅部署 DaemonSet"
	@echo "  make status         - 查看部署状态"
	@echo "  make logs           - 查看所有 Agent 日志"
	@echo "  make test-local     - 本地测试运行"
	@echo "  make clean          - 清理部署"
	@echo "  make clean-all      - 完全清理（包括 CRD）"
	@echo ""
