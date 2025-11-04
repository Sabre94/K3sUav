#!/bin/bash

# UAV Agent 部署脚本
# 构建 Docker 镜像并部署到 K3s 集群

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_DIR="$(dirname "$SCRIPT_DIR")"

IMAGE_NAME="uav-agent"
IMAGE_TAG="v0.1.0"
FULL_IMAGE="${IMAGE_NAME}:${IMAGE_TAG}"

echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo "  UAV Agent 部署脚本"
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo ""

cd "$PROJECT_DIR"

# 步骤 1: 检查 CRD
echo "📋 [1/5] 检查 UAVMetrics CRD..."
if kubectl get crd uavmetrics.uav.k3s.io &>/dev/null; then
    echo "  ✅ CRD 已存在"
else
    echo "  ⚠️  CRD 不存在，正在部署..."
    kubectl apply -f api/crd/uav-metrics-crd.yaml
    echo "  ✅ CRD 部署完成"
fi
echo ""

# 步骤 2: 构建 Docker 镜像
echo "🐳 [2/5] 构建 Docker 镜像..."
echo "  镜像名称: $FULL_IMAGE"

# 检查是否有 Docker 或者使用 K3s 的 ctr
if command -v docker &>/dev/null; then
    echo "  使用 Docker 构建..."
    docker build -t "$FULL_IMAGE" .
    echo "  ✅ Docker 镜像构建完成"

    # 将镜像导入 K3s
    echo "  导入镜像到 K3s..."
    docker save "$FULL_IMAGE" | sudo k3s ctr images import -
    echo "  ✅ 镜像已导入 K3s"

elif command -v buildah &>/dev/null; then
    echo "  使用 Buildah 构建..."
    buildah bud -t "$FULL_IMAGE" .
    buildah push "$FULL_IMAGE" "containers-storage:$FULL_IMAGE"
    echo "  ✅ Buildah 镜像构建完成"

else
    echo "  ⚠️  未找到 Docker 或 Buildah，尝试直接编译..."
    export PATH=$PATH:/usr/local/go/bin
    go build -o bin/uav-agent ./cmd/agent/
    echo "  ✅ 二进制文件已编译"
    echo "  ⚠️  注意: 需要手动构建镜像"
fi
echo ""

# 步骤 3: 部署 RBAC 和 ServiceAccount
echo "🔐 [3/5] 部署 RBAC 权限..."
kubectl apply -f deploy/agent-daemonset.yaml
echo "  ✅ RBAC 配置已部署"
echo ""

# 步骤 4: 等待 Pod 启动
echo "⏳ [4/5] 等待 Pod 启动..."
sleep 3

# 获取节点数量
NODE_COUNT=$(kubectl get nodes --no-headers | wc -l)
echo "  集群节点数: $NODE_COUNT"

# 等待所有 Pod 就绪
echo "  等待 DaemonSet Pod 启动（最多 60 秒）..."
for i in {1..60}; do
    READY_COUNT=$(kubectl get pods -l app=uav-agent -o jsonpath='{.items[*].status.conditions[?(@.type=="Ready")].status}' | grep -o "True" | wc -l)
    TOTAL_COUNT=$(kubectl get pods -l app=uav-agent --no-headers | wc -l)

    echo -ne "  进度: $READY_COUNT/$TOTAL_COUNT Pod 就绪\r"

    if [ "$READY_COUNT" -eq "$NODE_COUNT" ]; then
        echo -ne "\n"
        echo "  ✅ 所有 Pod 已就绪"
        break
    fi

    if [ $i -eq 60 ]; then
        echo -ne "\n"
        echo "  ⚠️  超时: 部分 Pod 未就绪"
    fi

    sleep 1
done
echo ""

# 步骤 5: 验证部署
echo "✅ [5/5] 验证部署..."

echo ""
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo "  Pod 状态"
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
kubectl get pods -l app=uav-agent -o wide

echo ""
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo "  UAVMetrics 资源"
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"

# 等待几秒让 Agent 创建 UAVMetrics
sleep 5
kubectl get uavmetrics -A

echo ""
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo "  部署完成！"
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo ""
echo "📊 查看实时数据:"
echo "  watch -n 2 'kubectl get uavmetrics -A'"
echo ""
echo "📝 查看 Agent 日志:"
echo "  kubectl logs -l app=uav-agent -f"
echo ""
echo "🔍 查看特定节点的详细数据:"
echo "  kubectl get uavmetrics uav-<node-name> -o yaml"
echo ""
echo "🗑️  卸载 Agent:"
echo "  kubectl delete -f deploy/agent-daemonset.yaml"
echo ""
