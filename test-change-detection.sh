#!/bin/bash
# 变化检测功能测试脚本

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
SIM_DATA_FILE="/data/sim/current.json"

echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo "  UAV Agent 变化检测测试"
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo ""

# 检查agent是否运行
echo "📋 检查Agent运行状态..."
POD_COUNT=$(kubectl get pods -l app=uav-agent-sim --field-selector=status.phase=Running --no-headers 2>/dev/null | wc -l)
if [ "$POD_COUNT" -eq 0 ]; then
    echo "  ⚠️  未找到运行中的Agent Pod"
    echo "  请先部署Agent: kubectl apply -f deploy/agent-daemonset-simulation-test.yaml"
    exit 1
fi
echo "  ✅ 找到 $POD_COUNT 个运行中的Agent Pod"
echo ""

# 选择一个Pod进行测试
TEST_POD=$(kubectl get pods -l app=uav-agent-sim --field-selector=status.phase=Running -o jsonpath='{.items[0].metadata.name}')
TEST_NODE=$(kubectl get pod $TEST_POD -o jsonpath='{.spec.nodeName}')
echo "📌 测试Pod: $TEST_POD (节点: $TEST_NODE)"
echo ""

# 显示当前配置
echo "⚙️  当前变化检测配置:"
kubectl get pod $TEST_POD -o json | jq -r '.spec.containers[0].env[] | select(.name | startswith("ENABLE_CHANGE") or startswith("POSITION_CHANGE") or startswith("BATTERY_CHANGE") or contains("UPDATE_INTERVAL")) | "  \(.name)=\(.value)"'
echo ""

# 测试场景1: 无变化（应该不推送）
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo "测试场景1: 无人机悬停（无变化）"
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo ""

cat > $SIM_DATA_FILE <<EOF
{
  "vm_id": "test_drone_001",
  "simulation_id": "change_detection_test",
  "timestamp": "2024-01-01T12:00:00Z",
  "time_step": 100,
  "position": { "x": 100.0, "y": 200.0, "z": 50.0 },
  "velocity": { "vx": 0.0, "vy": 0.0, "vz": 0.0 },
  "geodetic": {
    "latitude": 39.9042,
    "longitude": 116.4074,
    "altitude": 100.0
  },
  "battery_level": 0.85,
  "heading": 90.0,
  "altitude_agl": 50.0
}
EOF

echo "✓ 写入初始位置: (100, 200, 50), 电量: 85%"
echo "  等待15秒观察日志..."
sleep 15

# 检查推送情况
echo ""
echo "📊 最近15秒的推送记录:"
kubectl logs $TEST_POD --since=15s | grep -E "Metrics updated successfully|shouldUpdate=false" | tail -5
echo ""

# 测试场景2: 小幅移动（不应该推送）
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo "测试场景2: 小幅移动（< 5米阈值）"
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo ""

cat > $SIM_DATA_FILE <<EOF
{
  "vm_id": "test_drone_001",
  "simulation_id": "change_detection_test",
  "timestamp": "2024-01-01T12:01:00Z",
  "time_step": 110,
  "position": { "x": 102.0, "y": 201.0, "z": 50.0 },
  "velocity": { "vx": 0.5, "vy": 0.3, "vz": 0.0 },
  "geodetic": {
    "latitude": 39.9042,
    "longitude": 116.4074,
    "altitude": 100.0
  },
  "battery_level": 0.849,
  "heading": 90.0,
  "altitude_agl": 50.0
}
EOF

echo "✓ 移动到: (102, 201, 50) - 距离约2.2米"
echo "  预期: 不推送（< 5米阈值）"
echo "  等待10秒..."
sleep 10

echo ""
echo "📊 变化检测结果:"
kubectl logs $TEST_POD --since=10s | grep "Change detection result" | tail -3
echo ""

# 测试场景3: 大幅移动（应该推送）
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo "测试场景3: 大幅移动（> 5米阈值）"
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo ""

cat > $SIM_DATA_FILE <<EOF
{
  "vm_id": "test_drone_001",
  "simulation_id": "change_detection_test",
  "timestamp": "2024-01-01T12:02:00Z",
  "time_step": 120,
  "position": { "x": 110.0, "y": 205.0, "z": 50.0 },
  "velocity": { "vx": 2.0, "vy": 1.5, "vz": 0.0 },
  "geodetic": {
    "latitude": 39.9043,
    "longitude": 116.4075,
    "altitude": 100.0
  },
  "battery_level": 0.848,
  "heading": 95.0,
  "altitude_agl": 50.0
}
EOF

echo "✓ 移动到: (110, 205, 50) - 距离约10.3米"
echo "  预期: 推送（> 5米阈值）"
echo "  等待10秒..."
sleep 10

echo ""
echo "📊 推送记录:"
kubectl logs $TEST_POD --since=10s | grep "Metrics updated successfully" | tail -1
echo ""

# 测试场景4: 电量变化（应该推送）
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo "测试场景4: 电量下降（> 1%阈值）"
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo ""

cat > $SIM_DATA_FILE <<EOF
{
  "vm_id": "test_drone_001",
  "simulation_id": "change_detection_test",
  "timestamp": "2024-01-01T12:03:00Z",
  "time_step": 130,
  "position": { "x": 110.0, "y": 205.0, "z": 50.0 },
  "velocity": { "vx": 0.0, "vy": 0.0, "vz": 0.0 },
  "geodetic": {
    "latitude": 39.9043,
    "longitude": 116.4075,
    "altitude": 100.0
  },
  "battery_level": 0.838,
  "heading": 95.0,
  "altitude_agl": 50.0
}
EOF

echo "✓ 电量下降到: 83.8% (下降1.0%)"
echo "  预期: 推送（> 1%阈值）"
echo "  等待10秒..."
sleep 10

echo ""
echo "📊 推送记录:"
kubectl logs $TEST_POD --since=10s | grep "reason=.*battery" | tail -1
echo ""

# 显示统计信息
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo "📈 变化检测统计汇总"
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo ""
kubectl logs $TEST_POD | grep "Change detection statistics" | tail -1
echo ""

# 对比UAVMetrics更新情况
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo "📋 UAVMetrics CRD状态"
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo ""

UAV_RESOURCE="uav-${TEST_NODE}"
if kubectl get uavmetrics $UAV_RESOURCE &>/dev/null; then
    echo "当前位置:"
    kubectl get uavmetrics $UAV_RESOURCE -o jsonpath='{.spec.position}' | jq .
    echo ""
    echo "当前电量:"
    kubectl get uavmetrics $UAV_RESOURCE -o jsonpath='{.spec.battery.remainingPercent}'
    echo "%"
    echo ""
    echo "仿真信息:"
    kubectl get uavmetrics $UAV_RESOURCE -o jsonpath='{.spec.simulation}' | jq .
else
    echo "⚠️  UAVMetrics资源不存在: $UAV_RESOURCE"
fi

echo ""
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo "  测试完成！"
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo ""
echo "💡 提示:"
echo "  - 查看实时日志: kubectl logs $TEST_POD -f"
echo "  - 查看推送决策: kubectl logs $TEST_POD | grep 'Change detection result'"
echo "  - 查看统计数据: kubectl logs $TEST_POD | grep 'Change detection statistics'"
echo ""
