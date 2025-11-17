#!/bin/bash

# Pod 级别调度算法测试脚本

set -e

echo "================================================"
echo "  Pod 级别调度算法功能测试"
echo "================================================"
echo ""

# 检查 kubectl 是否可用
if ! command -v kubectl &> /dev/null; then
    echo "❌ kubectl 未安装或不可用"
    exit 1
fi

# 检查调度器是否运行
echo "📋 检查调度器状态..."
if kubectl get pods -l app=uav-scheduler -n uav-system &> /dev/null; then
    SCHEDULER_PODS=$(kubectl get pods -l app=uav-scheduler -n uav-system --no-headers | wc -l)
    if [ "$SCHEDULER_PODS" -eq 0 ]; then
        echo "❌ UAV Scheduler 未运行，请先部署调度器"
        exit 1
    fi
    echo "✅ UAV Scheduler 正在运行 (${SCHEDULER_PODS} 个 Pod)"
else
    echo "⚠️  无法访问 uav-system 命名空间，跳过检查"
fi

echo ""
echo "📋 检查 UAVMetrics CRD 数据..."
METRICS_COUNT=$(kubectl get uavmetrics -A --no-headers 2>/dev/null | wc -l)
if [ "$METRICS_COUNT" -eq 0 ]; then
    echo "⚠️  没有 UAVMetrics 数据，调度可能失败"
    echo "   请确保 UAV Agent 正在运行并采集数据"
else
    echo "✅ 找到 ${METRICS_COUNT} 个节点的 UAVMetrics 数据"
fi

echo ""
echo "================================================"
echo "  开始测试 Pod 级别算法选择"
echo "================================================"
echo ""

# 清理旧的测试 Pod
echo "🧹 清理旧的测试 Pod..."
kubectl delete pod video-processor long-running-task realtime-app balanced-app default-app 2>/dev/null || true
sleep 2

# 测试1: distance-based 算法
echo ""
echo "📍 测试 1: distance-based 算法"
echo "   部署 video-processor Pod..."
kubectl apply -f examples/test-pod-distance.yaml
echo "   ✅ 已提交"

sleep 2

# 测试2: battery-aware 算法
echo ""
echo "🔋 测试 2: battery-aware 算法"
echo "   部署 long-running-task Pod..."
kubectl apply -f examples/test-pod-battery.yaml
echo "   ✅ 已提交"

sleep 2

# 测试3: network-latency 算法
echo ""
echo "🌐 测试 3: network-latency 算法"
echo "   部署 realtime-app Pod..."
kubectl apply -f examples/test-pod-network.yaml
echo "   ✅ 已提交"

sleep 2

# 测试4: composite 算法
echo ""
echo "🎯 测试 4: composite 算法"
echo "   部署 balanced-app Pod..."
kubectl apply -f examples/test-pod-composite.yaml
echo "   ✅ 已提交"

sleep 2

# 测试5: 默认算法
echo ""
echo "⚙️  测试 5: 默认算法（无 annotation）"
echo "   部署 default-app Pod..."
kubectl apply -f examples/test-pod-default.yaml
echo "   ✅ 已提交"

echo ""
echo "⏳ 等待 Pod 调度完成（10秒）..."
sleep 10

# 显示调度结果
echo ""
echo "================================================"
echo "  调度结果"
echo "================================================"
echo ""

echo "📊 Pod 调度情况："
kubectl get pods -o wide | grep -E "NAME|video-processor|long-running-task|realtime-app|balanced-app|default-app" || echo "   未找到测试 Pod"

echo ""
echo "================================================"
echo "  调度器日志（最近 20 行）"
echo "================================================"
echo ""

if kubectl get pods -l app=uav-scheduler -n uav-system &> /dev/null; then
    kubectl logs -l app=uav-scheduler -n uav-system --tail=20 2>/dev/null | grep -E "Algorithm selected|scheduled successfully" || echo "   未找到相关日志"
else
    echo "⚠️  无法访问调度器日志"
fi

echo ""
echo "================================================"
echo "  详细信息查询命令"
echo "================================================"
echo ""
echo "查看 Pod 详细信息:"
echo "  kubectl get pods -o wide"
echo ""
echo "查看调度器实时日志:"
echo "  kubectl logs -l app=uav-scheduler -n uav-system -f"
echo ""
echo "查看特定 Pod 的调度事件:"
echo "  kubectl describe pod video-processor"
echo ""
echo "清理测试 Pod:"
echo "  kubectl delete pod video-processor long-running-task realtime-app balanced-app default-app"
echo ""
echo "================================================"
echo "  测试完成！"
echo "================================================"
